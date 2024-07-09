package operator

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vapi/rest"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	v1 "k8s.io/api/core/v1"
	vspherecfg "k8s.io/cloud-provider-vsphere/pkg/common/config"
	"k8s.io/klog/v2"

	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"github.com/openshift/vsphere-problem-detector/pkg/log"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
	"github.com/openshift/vsphere-problem-detector/pkg/version"
)

type vSphereCheckerInterface interface {
	runChecks(context.Context, *util.ClusterInfo) (*ResultCollector, error)
}

type vSphereChecker struct {
	controller *vSphereProblemDetectorController
}

var _ vSphereCheckerInterface = &vSphereChecker{}

func newVSphereChecker(c *vSphereProblemDetectorController) vSphereCheckerInterface {
	return &vSphereChecker{controller: c}
}

func (v *vSphereChecker) runChecks(ctx context.Context, clusterInfo *util.ClusterInfo) (*ResultCollector, error) {

	v.controller.metricsCollector.StartMetricCollection()

	resultCollector := NewResultsCollector()

	checkContext, err := v.connect(ctx)
	if err != nil {
		return resultCollector, err
	}

	defer func() {
		for _, vCenter := range checkContext.VCenters {
			if err := vCenter.GovmomiClient.Logout(ctx); err != nil {
				log.Logf("Failed to logout: %v", err)
			}
		}
	}()

	// Get the fully-qualified vsphere username
	authMgrs := make(map[string]check.AuthManager)
	userNames := make(map[string]string)
	for index := range checkContext.VCenters {
		vc := checkContext.VCenters[index]
		sessionMgr := session.NewManager(vc.VMClient)
		user, err := sessionMgr.UserSession(ctx)
		if err != nil {
			return resultCollector, fmt.Errorf("unable to run checks: %v", err)
		}

		authMgrs[vc.VCenterName] = object.NewAuthorizationManager(vc.VMClient)
		userNames[vc.VCenterName] = user.UserName
	}

	infra, err := v.controller.GetInfrastructure(ctx)
	if err != nil {
		return nil, err
	}

	checkContext.Context = ctx
	for index := range checkContext.VCenters {
		vc := checkContext.VCenters[index]
		vc.AuthManager = authMgrs[vc.VCenterName]
		vc.Username = userNames[vc.VCenterName]
	}
	checkContext.KubeClient = v.controller
	checkContext.ClusterInfo = clusterInfo
	checkContext.MetricsCollector = v.controller.metricsCollector

	check.ConvertToPlatformSpec(infra, checkContext)

	checkRunner := NewCheckThreadPool(parallelVSPhereCalls, channelBufferSize)

	v.enqueueClusterChecks(checkContext, checkRunner, resultCollector)
	if err := v.enqueueNodeChecks(checkContext, checkRunner, resultCollector); err != nil {
		v.controller.metricsCollector.FinishedAllChecks()
		return resultCollector, err
	}

	klog.V(4).Infof("Waiting for all checks")
	if err := checkRunner.Wait(ctx); err != nil {
		log.Logf("error waiting for metrics checks to finish: %v", err)
		v.controller.metricsCollector.FinishedAllChecks()
		return resultCollector, err
	}
	v.finishNodeChecks(checkContext)
	klog.Infof("Finished running all vSphere specific checks in the cluster")
	v.controller.metricsCollector.FinishedAllChecks()
	return resultCollector, nil
}

func (c *vSphereChecker) connect(ctx context.Context) (*check.CheckContext, error) {
	// use api infra as the basis of
	// variables instead of intree
	// external won't have these values...
	var checkContext *check.CheckContext

	cfgString, err := c.getVSphereConfig(ctx)
	if err != nil {
		return nil, err
	}

	// intree configuration
	cfg, err := parseConfig(cfgString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %s", err)
	}

	vCenters := make(map[string]*check.VCenter)
	for _, vCenter := range cfg.Config.VirtualCenter {
		username, password, err := c.getCredentials(vCenter.VCenterIP)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials to %s: %v", vCenter.VCenterIP, err)
		}

		vmClient, restClient, err := newClient(ctx, vCenter, username, password)
		if err != nil {
			if strings.Index(username, "\n") != -1 {
				syncErrrorMetric.WithLabelValues("UsernameWithNewLine").Set(1)
				return nil, fmt.Errorf("failed to connect to %s: username in credentials contains new line", vCenter.VCenterIP)
			} else {
				syncErrrorMetric.WithLabelValues("UsernameWithNewLine").Set(0)
			}

			if strings.Index(password, "\n") != -1 {
				syncErrrorMetric.WithLabelValues("PasswordWithNewLine").Set(1)
				return nil, fmt.Errorf("failed to connect to %s: password in credentials contains new line", vCenter.VCenterIP)
			} else {
				syncErrrorMetric.WithLabelValues("PasswordWithNewLine").Set(0)
			}
			syncErrrorMetric.WithLabelValues("InvalidCredentials").Set(1)
			return nil, fmt.Errorf("failed to connect to %s: %s", vCenter.VCenterIP, err)
		} else {
			syncErrrorMetric.WithLabelValues("InvalidCredentials").Set(0)
		}
		if _, ok := cfg.Config.VirtualCenter[vCenter.VCenterIP]; ok {
			cfg.Config.VirtualCenter[vCenter.VCenterIP].User = username
		}

		if !isValidvCenterUsernameWithDomain(username) {
			klog.Warningf("vCenter username for %s is without domain, please consider using username with full domain name", vCenter.VCenterIP)
			syncErrrorMetric.WithLabelValues("UsernameWithoutDomain").Set(1)
		} else {
			syncErrrorMetric.WithLabelValues("UsernameWithoutDomain").Set(0)
		}

		klog.V(2).Infof("Connected to %s as %s", vCenter.VCenterIP, username)

		vCenterInfo := check.VCenter{
			VCenterName:   vCenter.VCenterIP,
			TagManager:    vapitags.NewManager(restClient),
			GovmomiClient: vmClient,
			VMClient:      vmClient.Client,
			// Each check run gets its own cache
			Cache: cache.NewCheckCache(vmClient.Client),
		}

		vCenters[vCenterInfo.VCenterName] = &vCenterInfo
	}

	checkContext = &check.CheckContext{
		Context:  ctx,
		VMConfig: cfg,
		VCenters: vCenters,
	}

	return checkContext, nil
}

func (c *vSphereChecker) getCredentials(vCenterAddress string) (string, string, error) {
	secret, err := c.controller.secretLister.Secrets(operatorNamespace).Get(cloudCredentialsSecretName)
	if err != nil {
		return "", "", err
	}
	userKey := vCenterAddress + "." + "username"
	username, ok := secret.Data[userKey]
	if !ok {
		return "", "", fmt.Errorf("error parsing secret %q: key %q not found", cloudCredentialsSecretName, userKey)
	}
	passwordKey := vCenterAddress + "." + "password"
	password, ok := secret.Data[passwordKey]
	if !ok {
		return "", "", fmt.Errorf("error parsing secret %q: key %q not found", cloudCredentialsSecretName, passwordKey)
	}

	return string(username), string(password), nil
}

func (c *vSphereChecker) getVSphereConfig(ctx context.Context) (string, error) {
	infra, err := c.controller.infraLister.Get(infrastructureName)
	if err != nil {
		return "", err
	}
	if infra.Status.PlatformStatus == nil {
		return "", fmt.Errorf("unknown platform: infrastructure status.platformStatus is nil")
	}
	if infra.Status.PlatformStatus.Type != ocpv1.VSpherePlatformType {
		return "", fmt.Errorf("unsupported platform: %s", infra.Status.PlatformStatus.Type)
	}

	cloudConfigMap, err := c.controller.cloudConfigMapLister.ConfigMaps(util.CloudConfigNamespace).Get(infra.Spec.CloudConfig.Name)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster config: %s", err)
	}

	cfgString, found := cloudConfigMap.Data[infra.Spec.CloudConfig.Key]
	if !found {
		return "", fmt.Errorf("cluster config %s/%s does not contain key %q", util.CloudConfigNamespace, infra.Spec.CloudConfig.Name, infra.Spec.CloudConfig.Key)
	}
	klog.V(4).Infof("Got ConfigMap %s/%s with config:\n%s", util.CloudConfigNamespace, infra.Spec.CloudConfig.Name, cfgString)

	return cfgString, nil
}

func (c *vSphereChecker) enqueueClusterChecks(checkContext *check.CheckContext, checkRunner *CheckThreadPool, resultCollector *ResultCollector) {
	for name, checkFunc := range c.controller.clusterChecks {
		name := name
		checkFunc := checkFunc
		checkRunner.RunGoroutine(checkContext.Context, func() {
			runSingleClusterCheck(checkContext, name, checkFunc, resultCollector)
		})
	}
}

func (c *vSphereChecker) enqueueNodeChecks(checkContext *check.CheckContext, checkRunner *CheckThreadPool, resultCollector *ResultCollector) error {
	nodes, err := c.controller.ListNodes(checkContext.Context)
	if err != nil {
		return err
	}

	for _, nodeCheck := range c.controller.nodeChecks {
		nodeCheck.StartCheck()
	}

	for i := range nodes {
		node := nodes[i]
		c.enqueueSingleNodeChecks(checkContext, checkRunner, resultCollector, node)
	}
	return nil
}

func (c *vSphereChecker) enqueueSingleNodeChecks(checkContext *check.CheckContext, checkRunner *CheckThreadPool, resultCollector *ResultCollector, node *v1.Node) {
	// Run a go routine that reads VM from vSphere and schedules separate goroutines for each check.
	checkRunner.RunGoroutine(checkContext.Context, func() {
		// Try to get VM
		vm, err := getVM(checkContext, node)
		if err != nil {
			// mark all checks as failed
			for _, check := range c.controller.nodeChecks {
				res := checkResult{
					Name:  check.Name(),
					Error: err,
				}
				resultCollector.AddResult(res)
			}
			return
		}

		// We got the VM, enqueue all node checks
		for i := range c.controller.nodeChecks {
			check := c.controller.nodeChecks[i]
			klog.V(4).Infof("Adding node check %s:%s", node.Name, check.Name())
			runSingleNodeSingleCheck(checkContext, resultCollector, node, vm, check)
		}
	})
}

func runSingleClusterCheck(checkContext *check.CheckContext, name string, checkFunc check.ClusterCheck, resultCollector *ResultCollector) {
	res := checkResult{
		Name: name,
	}
	klog.V(4).Infof("%s starting", name)
	err := checkFunc(checkContext)
	if err != nil {
		res.Error = err
		clusterCheckErrrorMetric.WithLabelValues(name).Set(1)
		klog.V(2).Infof("%s failed: %s", name, err)
	} else {
		clusterCheckErrrorMetric.WithLabelValues(name).Set(0)
		klog.V(2).Infof("%s passed", name)
	}
	clusterCheckTotalMetric.WithLabelValues(name).Inc()
	resultCollector.AddResult(res)
}

func runSingleNodeSingleCheck(checkContext *check.CheckContext, resultCollector *ResultCollector, node *v1.Node, vm *mo.VirtualMachine, check check.NodeCheck) {
	name := check.Name()
	res := checkResult{
		Name: name,
	}
	klog.V(4).Infof("%s:%s starting ", name, node.Name)
	err := check.CheckNode(checkContext, node, vm)
	if err != nil {
		res.Error = err
		nodeCheckErrrorMetric.WithLabelValues(name, node.Name).Set(1)
		klog.V(2).Infof("%s:%s failed: %s", name, node.Name, err)
	} else {
		nodeCheckErrrorMetric.WithLabelValues(name, node.Name).Set(0)
		klog.V(2).Infof("%s:%s passed", name, node.Name)
	}
	nodeCheckTotalMetric.WithLabelValues(name, node.Name).Inc()
	resultCollector.AddResult(res)
}

func (c *vSphereChecker) finishNodeChecks(ctx *check.CheckContext) {
	for i := range c.controller.nodeChecks {
		check := c.controller.nodeChecks[i]
		check.FinishCheck(ctx)
	}
}

func getVM(checkContext *check.CheckContext, node *v1.Node) (*mo.VirtualMachine, error) {
	tctx, cancel := context.WithTimeout(checkContext.Context, *util.Timeout)
	defer cancel()

	vmUUID := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(node.Spec.ProviderID, "vsphere://")))
	if vmUUID == "" {
		return nil, fmt.Errorf("VMUUID is not set for node %s", node.Name)
	}

	vCenterInfo, err := check.GetVCenter(checkContext, node)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM for node %s: %s", node.Name, err)
	}

	// Find VM reference in the datastore, by UUID
	s := object.NewSearchIndex(vCenterInfo.VMClient)

	// datacenter can be nil...
	svm, err := s.FindByUuid(tctx, nil, vmUUID, true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM by UUID %s: %s", vmUUID, err)
	}

	if svm == nil {
		return nil, fmt.Errorf("failed to find VM by UUID %s", vmUUID)
	}

	// Find VM properties
	vm := object.NewVirtualMachine(vCenterInfo.VMClient, svm.Reference())

	var o mo.VirtualMachine
	tctx, cancel = context.WithTimeout(checkContext.Context, *util.Timeout)
	defer cancel()
	err = vm.Properties(tctx, vm.Reference(), check.NodeProperties, &o)
	if err != nil {
		return nil, fmt.Errorf("failed to load VM %s: %s", node.Name, err)
	}

	return &o, nil
}

func parseConfig(data string) (*util.VSphereConfig, error) {
	cfg := &util.VSphereConfig{}
	err := cfg.LoadConfig(data)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func newClient(ctx context.Context, cfg *vspherecfg.VirtualCenterConfig, username, password string) (*govmomi.Client, *rest.Client, error) {
	serverAddress := cfg.VCenterIP
	serverURL, err := soap.ParseURL(serverAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse config file: %s", err)
	}

	insecure := cfg.InsecureFlag

	tctx, cancel := context.WithTimeout(ctx, *util.Timeout)
	defer cancel()
	klog.V(4).Infof("Connecting to %s as %s, insecure %t", serverAddress, username, insecure)

	// Set user to nil there for prevent login during client creation.
	// See https://github.com/vmware/govmomi/blob/master/client.go#L91
	serverURL.User = nil
	client, err := govmomi.NewClient(tctx, serverURL, insecure)

	if err != nil {
		return nil, nil, err
	}

	// Set up user agent before login for being able to track vpdo component in vcenter sessions list
	vpdVersion := version.Get()
	client.UserAgent = fmt.Sprintf("vsphere-problem-detector/%s", vpdVersion)

	if err := client.Login(tctx, url.UserPassword(username, password)); err != nil {
		return nil, nil, fmt.Errorf("unable to login to vCenter: %w", err)
	}

	restClient := rest.NewClient(client.Client)
	if err := restClient.Login(tctx, url.UserPassword(username, password)); err != nil {
		client.Logout(context.TODO())
		return nil, nil, fmt.Errorf("unable to login to vCenter REST API: %w", err)
	}

	return client, restClient, nil
}

// isValidvCenterUsernameWithDomain checks whether username is valid or not.
// vSphere considers the username invalid if it does not contain a fully qualified domain name.
// This function was taken from the CSI driver code:
// Take from https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/a7e37ef11bad80693a250ab5a815b527d99e3401/pkg/common/config/config.go#L326-L336
func isValidvCenterUsernameWithDomain(username string) bool {
	regex := `^(?:[a-zA-Z0-9.-]+\\[a-zA-Z0-9._-]+|[a-zA-Z0-9._-]+@[a-zA-Z0-9.-]+)$`
	match, _ := regexp.MatchString(regex, username)
	return match
}
