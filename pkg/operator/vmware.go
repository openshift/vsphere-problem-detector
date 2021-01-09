package operator

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"gopkg.in/gcfg.v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/vsphere"
)

func (c *vSphereProblemDetectorController) connect(ctx context.Context) (*vsphere.VSphereConfig, *vim25.Client, error) {
	cfgString, err := c.getVSphereConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	cfg, err := c.parseConfig(cfgString)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse config: %s", err)
	}

	username, password, err := c.getCredentials(cfg)
	if err != nil {
		return nil, nil, err
	}

	vmClient, err := c.newClient(ctx, cfg, username, password)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %s", cfg.Workspace.VCenterIP, err)
	}
	klog.V(2).Infof("Connected to %s as %s", cfg.Workspace.VCenterIP, username)
	return cfg, vmClient.Client, nil
}

func (c *vSphereProblemDetectorController) connectPlainConfig(ctx context.Context, configFile string) (*vsphere.VSphereConfig, *vim25.Client, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, nil, err
	}
	cfg := &vsphere.VSphereConfig{}
	err = gcfg.ReadInto(cfg, file)
	if err != nil {
		return nil, nil, err
	}
	username := cfg.Global.User
	password := cfg.Global.Password

	vmClient, err := c.newClient(ctx, cfg, username, password)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %s", cfg.Workspace.VCenterIP, err)
	}
	klog.V(2).Infof("Connected to %s as %s", cfg.Workspace.VCenterIP, username)
	return cfg, vmClient.Client, nil
}

func (c *vSphereProblemDetectorController) parseConfig(data string) (*vsphere.VSphereConfig, error) {
	var cfg vsphere.VSphereConfig
	err := gcfg.ReadStringInto(&cfg, data)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *vSphereProblemDetectorController) newClient(ctx context.Context, cfg *vsphere.VSphereConfig, username, password string) (*govmomi.Client, error) {
	serverAddress := cfg.Workspace.VCenterIP
	serverURL, err := soap.ParseURL(serverAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %s", err)
	}
	serverURL.User = url.UserPassword(username, password)
	insecure := cfg.Global.InsecureFlag

	tctx, cancel := context.WithTimeout(ctx, *check.Timeout)
	defer cancel()
	klog.V(4).Infof("Connecting to %s as %s, insecure %t", serverAddress, username, insecure)
	client, err := govmomi.NewClient(tctx, serverURL, insecure)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (c *vSphereProblemDetectorController) getCredentials(cfg *vsphere.VSphereConfig) (string, string, error) {
	secret, err := c.secretLister.Secrets(operatorNamespace).Get(cloudCredentialsSecretName)
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

func (c *vSphereProblemDetectorController) getVSphereConfig(ctx context.Context) (string, error) {
	infra, err := c.infraLister.Get(infrastructureName)
	if err != nil {
		return "", err
	}
	if infra.Status.PlatformStatus == nil {
		return "", fmt.Errorf("unknown platform: infrastructure status.platformStatus is nil")
	}
	if infra.Status.PlatformStatus.Type != ocpv1.VSpherePlatformType {
		return "", fmt.Errorf("unsupported platform: %s", infra.Status.PlatformStatus.Type)
	}

	cloudConfigMap, err := c.cloudConfigMapLister.ConfigMaps(cloudConfigNamespace).Get(infra.Spec.CloudConfig.Name)
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

func (c *vSphereProblemDetectorController) getVM(checkContext *check.CheckContext, node *v1.Node) (*mo.VirtualMachine, error) {
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
