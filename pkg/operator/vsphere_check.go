package operator

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi/vapi/rest"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"net/url"
	"strings"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
	"github.com/openshift/vsphere-problem-detector/pkg/version"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"gopkg.in/gcfg.v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/vsphere"
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
	resultCollector := NewResultsCollector()
	vmConfig, vmClient, restClient, err := v.connect(ctx)
	if err != nil {
		return resultCollector, err
	}

	defer func() {
		if err := vmClient.Logout(ctx); err != nil {
			klog.Errorf("Failed to logout: %v", err)
		}
	}()

	// Get the fully-qualified vsphere username
	sessionMgr := session.NewManager(vmClient.Client)
	user, err := sessionMgr.UserSession(ctx)
	if err != nil {
		return resultCollector, err
	}

	authManager := object.NewAuthorizationManager(vmClient.Client)

	checkContext := &check.CheckContext{
		Context:     ctx,
		AuthManager: authManager,
		VMConfig:    vmConfig,
		VMClient:    vmClient.Client,
		TagManager:  vapitags.NewManager(restClient),
		Username:    user.UserName,
		KubeClient:  v.controller,
		ClusterInfo: clusterInfo,
		// Each check run gets its own cache
		Cache: check.NewCheckCache(vmClient.Client),
	}

	checkRunner := NewCheckThreadPool(parallelVSPhereCalls, channelBufferSize)

	v.enqueueClusterChecks(checkContext, checkRunner, resultCollector)
	if err := v.enqueueNodeChecks(checkContext, checkRunner, resultCollector); err != nil {
		return resultCollector, err
	}

	klog.V(4).Infof("Waiting for all checks")
	if err := checkRunner.Wait(ctx); err != nil {
		return resultCollector, err
	}
	v.finishNodeChecks(checkContext)
	return resultCollector, nil
}

func (c *vSphereChecker) connect(ctx context.Context) (*vsphere.VSphereConfig, *govmomi.Client, *rest.Client, error) {
	cfgString, err := c.getVSphereConfig(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := parseConfig(cfgString)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse config: %s", err)
	}

	username, password, err := c.getCredentials(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	vmClient, restClient, err := newClient(ctx, cfg, username, password)
	if err != nil {
		if strings.Index(username, "\n") != -1 {
			syncErrrorMetric.WithLabelValues("UsernameWithNewLine").Set(1)
			return nil, nil, nil, fmt.Errorf("failed to connect to %s: username in credentials contains new line", cfg.Workspace.VCenterIP)
		} else {
			syncErrrorMetric.WithLabelValues("UsernameWithNewLine").Set(0)
		}

		if strings.Index(password, "\n") != -1 {
			syncErrrorMetric.WithLabelValues("PasswordWithNewLine").Set(1)
			return nil, nil, nil, fmt.Errorf("failed to connect to %s: password in credentials contains new line", cfg.Workspace.VCenterIP)
		} else {
			syncErrrorMetric.WithLabelValues("PasswordWithNewLine").Set(0)
		}
		syncErrrorMetric.WithLabelValues("InvalidCredentials").Set(1)
		return nil, nil, nil, fmt.Errorf("failed to connect to %s: %s", cfg.Workspace.VCenterIP, err)
	} else {
		syncErrrorMetric.WithLabelValues("InvalidCredentials").Set(0)
	}
	if _, ok := cfg.VirtualCenter[cfg.Workspace.VCenterIP]; ok {
		cfg.VirtualCenter[cfg.Workspace.VCenterIP].User = username
	}

	klog.V(2).Infof("Connected to %s as %s", cfg.Workspace.VCenterIP, username)
	return cfg, vmClient, restClient, nil
}

func (c *vSphereChecker) getCredentials(cfg *vsphere.VSphereConfig) (string, string, error) {
	secret, err := c.controller.secretLister.Secrets(operatorNamespace).Get(cloudCredentialsSecretName)
	if err != nil {
		return "", "", err
	}
	userKey := cfg.Workspace.VCenterIP + "." + "username"
	username, ok := secret.Data[userKey]
	if !ok {
		return "", "", fmt.Errorf("error parsing secret %q: key %q not found", cloudCredentialsSecretName, userKey)
	}
	passwordKey := cfg.Workspace.VCenterIP + "." + "password"
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

	cloudConfigMap, err := c.controller.cloudConfigMapLister.ConfigMaps(cloudConfigNamespace).Get(infra.Spec.CloudConfig.Name)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster config: %s", err)
	}

	cfgString, found := cloudConfigMap.Data[infra.Spec.CloudConfig.Key]
	if !found {
		return "", fmt.Errorf("cluster config %s/%s does not contain key %q", cloudConfigNamespace, infra.Spec.CloudConfig.Name, infra.Spec.CloudConfig.Key)
	}
	klog.V(4).Infof("Got ConfigMap %s/%s with config:\n%s", cloudConfigNamespace, infra.Spec.CloudConfig.Name, cfgString)

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
	tctx, cancel := context.WithTimeout(checkContext.Context, *check.Timeout)
	defer cancel()

	// Find datastore
	finder := find.NewFinder(checkContext.VMClient, false)
	dc, err := finder.Datacenter(tctx, checkContext.VMConfig.Workspace.Datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to access Datacenter %s: %s", checkContext.VMConfig.Workspace.Datacenter, err)
	}

	// Find VM reference in the datastore, by UUID
	s := object.NewSearchIndex(dc.Client())
	vmUUID := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(node.Spec.ProviderID, "vsphere://")))
	tctx, cancel = context.WithTimeout(checkContext.Context, *check.Timeout)
	defer cancel()
	svm, err := s.FindByUuid(tctx, dc, vmUUID, true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM by UUID %s: %s", vmUUID, err)
	}
	if svm == nil {
		return nil, fmt.Errorf("unable to find VM by UUID %s", vmUUID)
	}

	// Find VM properties
	vm := object.NewVirtualMachine(checkContext.VMClient, svm.Reference())
	tctx, cancel = context.WithTimeout(checkContext.Context, *check.Timeout)
	defer cancel()
	var o mo.VirtualMachine
	err = vm.Properties(tctx, vm.Reference(), check.NodeProperties, &o)
	if err != nil {
		return nil, fmt.Errorf("failed to load VM %s: %s", node.Name, err)
	}

	return &o, nil
}

func parseConfig(data string) (*vsphere.VSphereConfig, error) {
	var cfg vsphere.VSphereConfig
	err := gcfg.ReadStringInto(&cfg, data)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func newClient(ctx context.Context, cfg *vsphere.VSphereConfig, username, password string) (*govmomi.Client, *rest.Client, error) {
	serverAddress := cfg.Workspace.VCenterIP
	serverURL, err := soap.ParseURL(serverAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse config file: %s", err)
	}

	insecure := cfg.Global.InsecureFlag

	tctx, cancel := context.WithTimeout(ctx, *check.Timeout)
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
