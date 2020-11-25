package operator

import (
	"context"
	"math"
	"time"

	operatorapi "github.com/openshift/api/operator/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	infrainformer "github.com/openshift/client-go/config/informers/externalversions/config/v1"
	infralister "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
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
	checks map[string]check.Check

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
		eventRecorder:        eventRecorder.WithComponentSuffix("vSphereProblemDetectorController"),
		checks:               check.DefaultChecks,
		backoff:              defaultBackoff,
	}
	return factory.New().WithSync(c.sync).WithSyncDegradedOnError(operatorClient).WithInformers(
		configInformer.Informer(),
		secretInformer.Informer(),
		cloudConfigMapInformer.Informer(),
	).ToController("vSphereProblemDetectorController", eventRecorder)
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

	// TODO: Run in a separate goroutine? We may not want to run time-consuming checks here.
	if time.Now().After(c.nextCheck) || c.lastResults == nil {
		delay, err := c.runChecks(ctx)
		if err != nil {
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
		Type:   operatorapi.OperatorStatusTypeAvailable,
		Status: operatorapi.ConditionTrue,
	}
	progressingCnd := operatorapi.OperatorCondition{
		Type:   operatorapi.OperatorStatusTypeProgressing,
		Status: operatorapi.ConditionFalse,
	}

	if _, _, updateErr := v1helpers.UpdateStatus(c.operatorClient,
		v1helpers.UpdateConditionFn(availableCnd),
		v1helpers.UpdateConditionFn(progressingCnd),
		resultConditionsFn(c.lastResults),
	); updateErr != nil {
		return updateErr
	}

	return nil
}

func resultConditionsFn(results []checkResult) v1helpers.UpdateStatusFunc {
	return func(status *operatorv1.OperatorStatus) error {
		for _, res := range results {
			st := operatorapi.ConditionTrue
			reason := ""
			if !res.Result {
				st = operatorapi.ConditionFalse
				reason = "CheckFailed"
			}
			cnd := operatorapi.OperatorCondition{
				Type:    res.Name + "OK",
				Status:  st,
				Message: res.Message,
				Reason:  reason,
			}
			v1helpers.SetOperatorCondition(&status.Conditions, cnd)
		}
		return nil
	}
}

func (c *vSphereProblemDetectorController) runChecks(ctx context.Context) (time.Duration, error) {
	vmConfig, vmClient, err := c.connect(ctx)
	if err != nil {
		return 0, err
	}

	var results []checkResult
	failed := false
	for name, checkFunc := range c.checks {
		res := checkResult{
			Name: name,
		}
		start := time.Now()
		err := checkFunc(ctx, vmConfig, vmClient, c)
		if err != nil {
			res.Result = false
			res.Message = err.Error()
			failed = true
			clusterCheckErrrorMetric.WithLabelValues(name).Inc()
			klog.V(2).Infof("Check %q failed: %s", name, err)
		} else {
			res.Result = true
			klog.V(42).Infof("Check %q passed", name)
		}
		duration := time.Now().Sub(start)
		clusterCheckMetric.WithLabelValues(name).Observe(duration.Seconds())
		results = append(results, res)
	}

	c.lastResults = results
	c.lastCheck = time.Now()
	var nextDelay time.Duration
	if failed {
		nextDelay = c.backoff.Step()
	} else {
		// Reset the backoff on success
		c.backoff = defaultBackoff
		// The next check after success is after the maximum backoff
		// (i.e. retry as slow as allowed).
		nextDelay = defaultBackoff.Cap
	}
	c.nextCheck = c.lastCheck.Add(nextDelay)
	klog.V(4).Infof("Scheduled the next check in %s (%s)", nextDelay, c.nextCheck)
	return nextDelay, nil
}
