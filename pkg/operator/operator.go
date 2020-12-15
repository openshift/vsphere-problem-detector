package operator

import (
	"context"
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
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
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
	Name  string
	Error error
}

const (
	controllerName             = "VSphereProblemDetectorController"
	infrastructureName         = "cluster"
	cloudCredentialsSecretName = "vsphere-cloud-credentials"
	// TODO: make it configurable?
	parallelVSPhereCalls = 10
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

	checkContext := &check.CheckContext{
		Context:    ctx,
		VMConfig:   vmConfig,
		VMClient:   vmClient,
		KubeClient: c,
	}

	checkRunner := NewCheckThreadPool(parallelVSPhereCalls)
	resultCollector := NewResultsCollector()
	var errs []error
	if err := c.enqueueClusterChecks(checkContext, checkRunner, resultCollector); err != nil {
		errs = append(errs, err)
	}
	if err := c.enqueueNodeChecks(checkContext, checkRunner, resultCollector); err != nil {
		errs = append(errs, err)
	}

	klog.V(4).Infof("Waiting for all checks")
	if err := checkRunner.Wait(ctx); err != nil {
		return 0, err
	}

	klog.V(4).Infof("All checks complete")

	results, resultErrors := resultCollector.Collect()
	c.reportResults(results)
	c.lastResults = results
	c.lastCheck = time.Now()
	var nextDelay time.Duration
	// Join errors from add*Check and all result errors to determine the next check delay
	finalErr := errors.NewAggregate(append(errs, resultErrors...))
	if finalErr != nil {
		// Use exponential backoff
		nextDelay = c.backoff.Step()
	} else {
		// Reset the backoff on success
		c.backoff = defaultBackoff
		// Delay after success is after the maximum backoff
		// (i.e. retry as slow as allowed).
		nextDelay = defaultBackoff.Cap
	}
	c.nextCheck = c.lastCheck.Add(nextDelay)
	klog.V(2).Infof("Scheduled the next check in %s (%s)", nextDelay, c.nextCheck)
	return nextDelay, nil
}

func (c *vSphereProblemDetectorController) enqueueClusterChecks(checkContext *check.CheckContext, checkRunner *CheckThreadPool, resultCollector *ResultCollector) error {
	for name, checkFunc := range c.clusterChecks {
		name := name
		checkFunc := checkFunc
		checkRunner.RunGoroutine(checkContext.Context, func() {
			c.runSingleClusterCheck(checkContext, name, checkFunc, resultCollector)
		})
	}
	return nil
}

func (c *vSphereProblemDetectorController) runSingleClusterCheck(checkContext *check.CheckContext, name string, checkFunc check.ClusterCheck, resultCollector *ResultCollector) {
	res := checkResult{
		Name: name,
	}
	klog.V(4).Infof("%s starting", name)
	err := checkFunc(checkContext)
	if err != nil {
		res.Error = err
		clusterCheckErrrorMetric.WithLabelValues(name).Inc()
		klog.V(2).Infof("%s failed: %s", name, err)
	} else {
		klog.V(2).Infof("%s passed", name)
	}
	clusterCheckTotalMetric.WithLabelValues(name).Inc()
	resultCollector.AddResult(res)
}

func (c *vSphereProblemDetectorController) enqueueNodeChecks(checkContext *check.CheckContext, checkRunner *CheckThreadPool, resultCollector *ResultCollector) error {
	nodes, err := c.ListNodes(checkContext.Context)
	if err != nil {
		return err
	}

	for _, nodeCheck := range c.nodeChecks {
		nodeCheck.StartCheck()
	}

	for i := range nodes {
		node := &nodes[i]
		c.enqueueSingleNodeChecks(checkContext, checkRunner, resultCollector, node)
	}
	return nil
}

func (c *vSphereProblemDetectorController) enqueueSingleNodeChecks(checkContext *check.CheckContext, checkRunner *CheckThreadPool, resultCollector *ResultCollector, node *v1.Node) {
	// Run a go routine that reads VM from vSphere and schedules separate goroutines for each check.
	checkRunner.RunGoroutine(checkContext.Context, func() {
		// Try to get VM
		vm, err := c.getVM(checkContext, node)
		if err != nil {
			// mark all checks as failed
			for _, check := range c.nodeChecks {
				res := checkResult{
					Name:  check.Name(),
					Error: err,
				}
				resultCollector.AddResult(res)
			}
			return
		}
		// We got the VM, enqueue all node checks
		for i := range c.nodeChecks {
			check := c.nodeChecks[i]
			klog.V(4).Infof("Adding node check %s:%s", node.Name, check.Name())
			checkRunner.RunGoroutine(checkContext.Context, func() {
				c.runSingleNodeSingleCheck(checkContext, resultCollector, node, vm, check)
			})
		}
	})
}

func (c *vSphereProblemDetectorController) runSingleNodeSingleCheck(checkContext *check.CheckContext, resultCollector *ResultCollector, node *v1.Node, vm *mo.VirtualMachine, check check.NodeCheck) {
	name := check.Name()
	res := checkResult{
		Name: name,
	}
	klog.V(4).Infof("%s:%s starting ", name, node.Name)
	err := check.CheckNode(checkContext, node, vm)
	if err != nil {
		res.Error = err
		nodeCheckErrrorMetric.WithLabelValues(name, node.Name).Inc()
		klog.V(2).Infof("%s:%s failed: %s", name, node.Name, err)
	} else {
		klog.V(2).Infof("%s:%s passed", name, node.Name)
	}
	nodeCheckTotalMetric.WithLabelValues(name, node.Name).Inc()
	resultCollector.AddResult(res)
}

// reportResults sends events for all checks.
func (c *vSphereProblemDetectorController) reportResults(results []checkResult) {
	for _, res := range results {
		if res.Error != nil {
			c.eventRecorder.Warningf("FailedVSphere"+res.Name+"Failed", res.Error.Error())
		} else {
			c.eventRecorder.Eventf("SucceededVSphere"+res.Name, "Check succeeded")
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
