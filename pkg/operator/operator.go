package operator

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver"
	ocpv1 "github.com/openshift/api/config/v1"
	operatorapi "github.com/openshift/api/operator/v1"
	infrainformer "github.com/openshift/client-go/config/informers/externalversions/config/v1"
	infralister "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	storagelister "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
)

type vSphereProblemDetectorController struct {
	operatorClient          *OperatorClient
	kubeClient              kubernetes.Interface
	infraLister             infralister.InfrastructureLister
	secretLister            corelister.SecretLister
	nodeLister              corelister.NodeLister
	pvLister                corelister.PersistentVolumeLister
	scLister                storagelister.StorageClassLister
	cloudConfigMapLister    corelister.ConfigMapLister
	operatorConfigMapLister corelister.ConfigMapLister
	eventRecorder           events.Recorder

	// List of checks to perform (useful for unit-tests: replace with a dummy check).
	clusterChecks map[string]check.ClusterCheck
	nodeChecks    []check.NodeCheck
	checkerFunc   func(c *vSphereProblemDetectorController) vSphereCheckerInterface

	lastCheck time.Time
	nextCheck time.Time
	backoff   wait.Backoff
}

type clusterCheckResult struct {
	checkError         error
	blockUpgrade       bool
	blockUpgradeReason string
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
	// Size of golang channel buffer
	channelBufferSize     = 100
	minHostVersion        = "6.7.3"
	minVCenterVersion     = "6.7.3"
	hardwareVersionPrefix = "vmx-"
	minHardwareVersion    = 15
)

var (
	defaultBackoff = wait.Backoff{
		Duration: time.Minute,
		Factor:   2,
		Jitter:   0.01,
		// Don't limit nr. of steps
		Steps: math.MaxInt32,
		// Maximum interval between checks.
		Cap: time.Hour * 1,
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
	operatorConfigMapInformer := namespacedInformer.InformersFor(operatorNamespace).Core().V1().ConfigMaps()
	nodeInformer := namespacedInformer.InformersFor("").Core().V1().Nodes()
	pvInformer := namespacedInformer.InformersFor("").Core().V1().PersistentVolumes()
	scInformer := namespacedInformer.InformersFor("").Storage().V1().StorageClasses()
	c := &vSphereProblemDetectorController{
		operatorClient:          operatorClient,
		kubeClient:              kubeClient,
		secretLister:            secretInformer.Lister(),
		nodeLister:              nodeInformer.Lister(),
		pvLister:                pvInformer.Lister(),
		scLister:                scInformer.Lister(),
		cloudConfigMapLister:    cloudConfigMapInformer.Lister(),
		operatorConfigMapLister: operatorConfigMapInformer.Lister(),
		infraLister:             configInformer.Lister(),
		eventRecorder:           eventRecorder.WithComponentSuffix(controllerName),
		clusterChecks:           check.DefaultClusterChecks,
		nodeChecks:              check.DefaultNodeChecks,
		backoff:                 defaultBackoff,
		checkerFunc:             newVSphereChecker,
		nextCheck:               time.Time{}, // Explicitly set to zero to run checks on the first sync().
	}
	return factory.New().WithSync(c.sync).WithSyncDegradedOnError(operatorClient).WithInformers(
		configInformer.Informer(),
		secretInformer.Informer(),
		nodeInformer.Informer(),
		pvInformer.Informer(),
		scInformer.Informer(),
		cloudConfigMapInformer.Informer(),
		operatorConfigMapInformer.Informer(),
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
	if !platformSupported {
		return nil
	}

	cfg, err := ParseConfigMap(c.operatorConfigMapLister)
	if err != nil {
		return err
	}
	if cfg.AlertsDisabled {
		alertsDisabledMetric.Set(1)
	} else {
		alertsDisabledMetric.Set(0)
	}
	if cfg.Disabled {
		// disable all alerts when the detector itself is disabled
		alertsDisabledMetric.Set(1)
		klog.V(4).Infof("vsphere-problem-detector is disabled via ConfigMap")
		// Reset all conditions
		return c.updateConditions(ctx, clusterCheckResult{
			checkError:   nil,
			blockUpgrade: false,
		})
	}

	clusterInfo := util.NewClusterInfo()
	delay, lastCheckResult, checkPerformed := c.runSyncChecks(ctx, clusterInfo)

	// if no checks were performed don't update conditons
	if !checkPerformed {
		return nil
	}

	// update conditions when checks are performed
	queue := syncCtx.Queue()
	queueKey := syncCtx.QueueKey()
	c.nextCheck = c.lastCheck.Add(delay)
	klog.V(2).Infof("Scheduled the next check in %s (%s)", delay, c.nextCheck)
	time.AfterFunc(delay, func() {
		queue.Add(queueKey)
	})
	return c.updateConditions(ctx, lastCheckResult)
}

func (c *vSphereProblemDetectorController) updateConditions(ctx context.Context, lastCheckResult clusterCheckResult) error {
	availableCnd := operatorapi.OperatorCondition{
		Type:   controllerName + operatorapi.OperatorStatusTypeAvailable,
		Status: operatorapi.ConditionTrue,
	}

	if lastCheckResult.checkError != nil {
		// E.g.: "failed to connect to vcenter.example.com: ServerFaultCode: Cannot complete login due to an incorrect user name or password."
		availableCnd.Message = lastCheckResult.checkError.Error()
		availableCnd.Reason = "SyncFailed"
	}

	updateFuncs := []v1helpers.UpdateStatusFunc{}
	updateFuncs = append(updateFuncs, v1helpers.UpdateConditionFn(availableCnd))
	if _, _, updateErr := v1helpers.UpdateStatus(ctx, c.operatorClient, updateFuncs...); updateErr != nil {
		return updateErr
	}
	return nil
}

// runSyncChecks runs vsphere checks and return next duration and whether checks were actually ran.
func (c *vSphereProblemDetectorController) runSyncChecks(ctx context.Context, clusterInfo *util.ClusterInfo) (time.Duration, clusterCheckResult, bool) {
	var delay time.Duration
	var lastCheckResult clusterCheckResult
	if !time.Now().After(c.nextCheck) {
		return delay, lastCheckResult, false
	}

	delay, err := c.runChecks(ctx, clusterInfo)
	if err != nil {
		klog.Errorf("failed to run checks: %s", err)
		lastCheckResult.checkError = err
		syncErrrorMetric.WithLabelValues("SyncError").Set(1)
	} else {
		syncErrrorMetric.WithLabelValues("SyncError").Set(0)
	}

	lastCheckResult.blockUpgrade, lastCheckResult.blockUpgradeReason = c.checkForDeprecation(clusterInfo)
	// if we are going to block upgrades but there was no error
	// then we should try more frequently in case node/cluster status is updated
	if lastCheckResult.blockUpgrade && err == nil {
		delay = c.backoff.Step()
	}
	return delay, lastCheckResult, true
}

func (c *vSphereProblemDetectorController) checkForDeprecation(clusterInfo *util.ClusterInfo) (bool, string) {
	esxiVersions := clusterInfo.GetHostVersions()
	for host, esxiVersion := range esxiVersions {
		hasMinimum, err := isMinimumVersion(minHostVersion, esxiVersion.APIVersion)
		if err != nil {
			klog.Errorf("error parsing host version: %v", err)
			continue
		}
		if !hasMinimum {
			return true, fmt.Sprintf("host %s is on esxi version %s", host, esxiVersion.APIVersion)
		}
	}

	for hwVersion := range clusterInfo.GetHardwareVersion() {
		vmHWVersion := strings.Trim(hwVersion, hardwareVersionPrefix)
		versionInt, err := strconv.ParseInt(vmHWVersion, 0, 64)
		if err != nil {
			klog.Errorf("error parsing hardware version %s: %v", hwVersion, err)
			continue
		}
		if versionInt < minHardwareVersion {
			return true, fmt.Sprintf("one or more VMs are on hardware version %s", hwVersion)
		}
	}

	_, vcenterAPIVersion := clusterInfo.GetVCenterVersion()
	hasMinimum, err := isMinimumVersion(minVCenterVersion, vcenterAPIVersion)
	if err != nil {
		klog.Errorf("error parsing vcenter version: %v", err)
	}

	if !hasMinimum {
		return true, fmt.Sprintf("connected vcenter is on %s version", vcenterAPIVersion)
	}

	return false, ""
}

func isMinimumVersion(minimumVersion string, currentVersion string) (bool, error) {
	minimumSemver, err := semver.New(minimumVersion)
	if err != nil {
		return true, err
	}
	semverString := parseForSemver(currentVersion)
	currentSemVer, err := semver.ParseTolerant(semverString)
	if err != nil {
		return true, err
	}
	if currentSemVer.Compare(*minimumSemver) >= 0 {
		return true, nil
	}
	return false, nil
}

func parseForSemver(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) > 3 {
		return strings.Join(parts[0:3], ".")
	}
	return version
}

func (c *vSphereProblemDetectorController) runChecks(ctx context.Context, clusterInfo *util.ClusterInfo) (time.Duration, error) {
	// pre-calculate exp. backoff on error
	nextErrorDelay := c.backoff.Step()
	c.lastCheck = time.Now()

	checker := c.checkerFunc(c)
	resultCollector, err := checker.runChecks(ctx, clusterInfo)
	if err != nil {
		return nextErrorDelay, err
	}
	klog.V(4).Infof("All checks complete")

	results, checkError := resultCollector.Collect()
	c.reportResults(results)
	var nextDelay time.Duration
	if checkError != nil {
		// Use exponential backoff
		nextDelay = nextErrorDelay
	} else {
		// Reset the backoff on success
		c.backoff = defaultBackoff
		// Delay after success is after the maximum backoff
		// (i.e. retry as slow as allowed).
		nextDelay = defaultBackoff.Cap
	}
	return nextDelay, checkError
}

// reportResults sends events for all checks.
func (c *vSphereProblemDetectorController) reportResults(results []checkResult) {
	for _, res := range results {
		if res.Error != nil {
			c.eventRecorder.Warningf("FailedVSphere"+res.Name, res.Error.Error())
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
