package operator

import (
	"context"
	"fmt"
	"math"
	"time"

	ocpv1 "github.com/openshift/api/config/v1"
	operatorapi "github.com/openshift/api/operator/v1"
	infrainformer "github.com/openshift/client-go/config/informers/externalversions/config/v1"
	infralister "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

type vSphereProblemDetectorController struct {
	operatorClient       *OperatorClient
	kubeClient           kubernetes.Interface
	infraLister          infralister.InfrastructureLister
	secretLister         corelister.SecretLister
	cloudConfigMapLister corelister.ConfigMapLister
	eventRecorder        events.Recorder

	// List of checks to perform (useful for unit-tests: replace with a dummy check).
	clusterChecks map[string]check.ClusterCheck
	nodeChecks    []check.NodeCheck

	lastCheck   time.Time
	nextCheck   time.Time
	lastResults []checkResult
	backoff     wait.Backoff
}

type checkResult struct {
	Name    string
	Result  bool
	Message string
}

const (
	controllerName             = "VSphereProblemDetectorController"
	infrastructureName         = "cluster"
	cloudCredentialsSecretName = "vsphere-cloud-credentials"
)

var (
	defaultBackoff = wait.Backoff{
		Duration: time.Minute,
		Factor:   2,
		Jitter:   0.01,
		// Don't limit nr. of steps
		Steps: math.MaxInt32,
		// Maximum interval between checks.
		Cap: time.Hour * 8,
	}
)

func NewVSphereProblemDetectorController(
	operatorClient *OperatorClient,
	kubeClient kubernetes.Interface,
	namespacedInformer v1helpers.KubeInformersForNamespaces,
	configInformer infrainformer.InfrastructureInformer,
	eventRecorder events.Recorder) factory.Controller {

	secretInformer := namespacedInformer.InformersFor(operatorNamespace).Core().V1().Secrets()
	cloudConfigMapInformer := namespacedInformer.InformersFor(cloudConfigNamespace).Core().V1().ConfigMaps()
	c := &vSphereProblemDetectorController{
		operatorClient:       operatorClient,
		kubeClient:           kubeClient,
		secretLister:         secretInformer.Lister(),
		cloudConfigMapLister: cloudConfigMapInformer.Lister(),
		infraLister:          configInformer.Lister(),
		eventRecorder:        eventRecorder.WithComponentSuffix(controllerName),
		clusterChecks:        check.DefaultClusterChecks,
		nodeChecks:           check.DefaultNodeChecks,
		backoff:              defaultBackoff,
		nextCheck:            time.Time{}, // Explicitly set to zero to run checks on the first sync().
	}
	return factory.New().WithSync(c.sync).WithSyncDegradedOnError(operatorClient).WithInformers(
		configInformer.Informer(),
		secretInformer.Informer(),
		cloudConfigMapInformer.Informer(),
	).ToController(controllerName, c.eventRecorder)
}

func (c *vSphereProblemDetectorController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	klog.V(4).Infof("vSphereProblemDetectorController.Sync started")
	defer klog.V(4).Infof("vSphereProblemDetectorController.Sync finished")

	opSpec, _, _, err := c.operatorClient.GetOperatorState()
	if err != nil {
		return err
	}
	if opSpec.ManagementState != operatorapi.Managed {
		return nil
	}

	platformSupported, err := c.platformSupported()
	if err != nil {
		return err
	}

	// TODO: Run in a separate goroutine? We may not want to run time-consuming checks here.
	if platformSupported && time.Now().After(c.nextCheck) {
		delay, err := c.runChecks(ctx)
		if err != nil {
			// This sets VSphereProblemDetectorControllerDegraded condition
			return err
		}
		// Poke the controller sync loop after the delay to re-run tests
		queue := syncCtx.Queue()
		queueKey := syncCtx.QueueKey()
		time.AfterFunc(delay, func() {
			queue.Add(queueKey)
		})
	}

	availableCnd := operatorapi.OperatorCondition{
		Type:   controllerName + operatorapi.OperatorStatusTypeAvailable,
		Status: operatorapi.ConditionTrue,
	}

	if _, _, updateErr := v1helpers.UpdateStatus(c.operatorClient,
		v1helpers.UpdateConditionFn(availableCnd),
	); updateErr != nil {
		return updateErr
	}

	return nil
}

func (c *vSphereProblemDetectorController) runChecks(ctx context.Context) (time.Duration, error) {
	vmConfig, vmClient, err := c.connect(ctx)
	if err != nil {
		return 0, err
	}

	var results []checkResult
	checkContext := &check.CheckContext{
		Context:    ctx,
		VMConfig:   vmConfig,
		VMClient:   vmClient,
		KubeClient: c,
	}

	var errs []error
	clusterResults, err := c.runClusterChecks(checkContext)
	if err != nil {
		errs = append(errs, err)
	}
	if clusterResults != nil {
		results = append(results, clusterResults...)
	}

	nodeResults, err := c.runNodeChecks(checkContext)
	if err != nil {
		errs = append(errs, err)
	}
	if nodeResults != nil {
		results = append(results, nodeResults...)
	}

	c.reportResults(results)
	c.lastResults = results
	c.lastCheck = time.Now()
	var nextDelay time.Duration
	finalErr := errors.NewAggregate(errs)
	if finalErr != nil {
		nextDelay = c.backoff.Step()
	} else {
		// Reset the backoff on success
		c.backoff = defaultBackoff
		// The next check after success is after the maximum backoff
		// (i.e. retry as slow as allowed).
		nextDelay = defaultBackoff.Cap
	}
	c.nextCheck = c.lastCheck.Add(nextDelay)
	klog.V(2).Infof("Scheduled the next check in %s (%s)", nextDelay, c.nextCheck)
	return nextDelay, nil
}

func (c *vSphereProblemDetectorController) runClusterChecks(checkContext *check.CheckContext) ([]checkResult, error) {
	var errs []error
	var results []checkResult
	for name, checkFunc := range c.clusterChecks {
		res := checkResult{
			Name: name,
		}
		klog.V(4).Infof("%s starting", name)
		err := checkFunc(checkContext)
		if err != nil {
			res.Result = false
			res.Message = err.Error()
			errs = append(errs, err)
			clusterCheckErrrorMetric.WithLabelValues(name).Inc()
			klog.V(2).Infof("%s failed: %s", name, err)
		} else {
			res.Result = true
			klog.V(2).Infof("%s passed", name)
		}
		clusterCheckTotalMetric.WithLabelValues(name).Inc()
		results = append(results, res)
	}

	return results, errors.NewAggregate(errs)
}

func (c *vSphereProblemDetectorController) runNodeChecks(checkContext *check.CheckContext) ([]checkResult, error) {
	nodes, err := c.ListNodes(checkContext.Context)
	if err != nil {
		return nil, err
	}

	// Name of check -> array of errors of the check (one for each node where it failed).
	checkErrors := make(map[string][]error)
	for _, nodeCheck := range c.nodeChecks {
		checkErrors[nodeCheck.Name()] = []error{}
		nodeCheck.StartCheck()
	}

	for i := range nodes {
		node := &nodes[i]
		vm, vmErr := c.getVM(checkContext, node)
		// vmErr will be processed later to make all checks fail

		for _, check := range c.nodeChecks {
			var err error
			name := check.Name()
			if vmErr == nil {
				klog.V(4).Infof("%s:%s starting ", name, node.Name)
				err = check.CheckNode(checkContext, node, vm)
			} else {
				// Now use vmErr to mark all checks as failed with the same error
				err = vmErr
			}
			if err != nil {
				checkErrors[name] = append(checkErrors[name], fmt.Errorf("%s: %s", node.Name, err))
				nodeCheckErrrorMetric.WithLabelValues(name, node.Name).Inc()
				klog.V(2).Infof("%s:%s failed: %s", name, node.Name, err)
			} else {
				klog.V(2).Infof("%s:%s passed", name, node.Name)
			}
			nodeCheckTotalMetric.WithLabelValues(name, node.Name).Inc()
		}
	}
	// Finish all node checks - this may set new metrics
	for _, check := range c.nodeChecks {
		check.FinishCheck()
	}

	// Convert the errors to checkResults
	var results []checkResult
	var allErrors []error
	for _, nodeCheck := range c.nodeChecks {
		name := nodeCheck.Name()
		errs := checkErrors[name]
		res := checkResult{
			Name: name,
		}
		if len(errs) != 0 {
			res.Result = false
			res.Message = check.JoinErrors(errs).Error()
			allErrors = append(allErrors, errs...)
		} else {
			res.Result = true
		}
		results = append(results, res)
	}
	return results, errors.NewAggregate(allErrors)
}

// reportResults sends events for all checks.
func (c *vSphereProblemDetectorController) reportResults(results []checkResult) {
	for _, res := range results {
		if res.Result {
			c.eventRecorder.Eventf("SucceededVSphere"+res.Name, res.Message)
		} else {
			c.eventRecorder.Warningf("FailedVSphere"+res.Name+"Failed", res.Message)
		}
	}
}

func (c *vSphereProblemDetectorController) platformSupported() (bool, error) {
	infra, err := c.infraLister.Get(infrastructureName)
	if err != nil {
		return false, err
	}

	if infra.Status.PlatformStatus == nil {
		klog.V(4).Infof("Unknown platform: infrastructure status.platformStatus is nil")
		return false, nil
	}
	if infra.Status.PlatformStatus.Type != ocpv1.VSpherePlatformType {
		klog.V(4).Infof("Unsupported platform: %s", infra.Status.PlatformStatus.Type)
		return false, nil
	}
	return true, nil
}
