package testlib

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"os"
	"path"
	goruntime "runtime"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/rest"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/vsphere-problem-detector/pkg/util"

	// required to initialize the VAPI endpoint.
	_ "github.com/vmware/govmomi/vapi/simulator"
)

//go:embed *.yaml *.ini
var f embed.FS

const (
	DefaultModel    = "testlib/testdata/default"
	HostGroupModel  = "testlib/testdata/host-group"
	defaultDC       = "DC0"
	defaultVMPath   = "/DC0/vm/"
	defaultHost     = "H0"
	DefaultHostId   = "host-24" // Generated by vcsim
	defaultHostPath = "/DC0/host/DC0_"
)

var (
	NodeProperties = []string{"config.extraConfig", "config.flags", "config.version", "runtime.host"}
)

type SimulatedVM struct {
	Name, UUID string
}

var (
	// Virtual machines generated by vSphere simulator. UUIDs look generated, but they're stable.
	defaultVMs = []SimulatedVM{
		{"DC0_H0_VM0", "265104de-1472-547c-b873-6dc7883fb6cb"},
		{"DC0_H0_VM1", "12f8928d-f144-5c57-89db-dd2d0902c9fa"},
	}
)

func init() {
	_, filename, _, _ := goruntime.Caller(0)
	dir := path.Join(path.Dir(filename), "..")
	err := os.Chdir(dir)
	if err != nil {
		panic(err)
	}
}

func connectToSimulator(s *simulator.Server) (*vim25.Client, error) {
	client, err := govmomi.NewClient(context.TODO(), s.URL, true)
	if err != nil {
		return nil, err
	}
	return client.Client, nil
}

func GetVSphereConfig(fileName string) *util.VSphereConfig {
	var cfg util.VSphereConfig
	err := cfg.LoadConfig(getVSphereConfigString(fileName))
	if err != nil {
		panic(err)
	}
	return &cfg
}

func getVSphereConfigString(fileName string) string {
	if fileName == "" {
		fileName = "simple_config.ini"
	}
	data, err := ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// ReadFile reads and returns the content of the named file.
func ReadFile(name string) ([]byte, error) {
	return f.ReadFile(name)
}

type TestSetup struct {
	Context     context.Context
	VMConfig    *util.VSphereConfig
	VCenters    map[string]*VCenter
	ClusterInfo *util.ClusterInfo
}

type VCenter struct {
	VMClient   *vim25.Client
	TagManager *vapitags.Manager
	Username   string
	Model      simulator.Model
}

func SetupSimulator(kubeClient *FakeKubeClient, modelDir string) (setup *TestSetup, cleanup func(), err error) {
	return SetupSimulatorWithConfig(kubeClient, modelDir, "")
}

func SetupSimulatorWithConfig(kubeClient *FakeKubeClient, modelDir, cloudConfig string) (setup *TestSetup, cleanup func(), err error) {
	// Load config
	vConfig := GetVSphereConfig(cloudConfig)
	vcs := make(map[string]*VCenter)
	var sessions []*simulator.Server
	var models []*simulator.Model

	// For now, we'll load the same model data for each vCenter, but just have each vCenter reference different datacenter,
	// datastore, etc.
	for vCenterName := range vConfig.Config.VirtualCenter {
		model := simulator.Model{}
		err = model.Load(modelDir)
		if err != nil {
			return nil, nil, err
		}

		model.Service.TLS = new(tls.Config)
		model.Service.RegisterEndpoints = true

		s := model.Service.NewServer()
		client, err := connectToSimulator(s)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to connect to the simulator: %s", err)
		}

		sessionMgr := session.NewManager(client)
		userSession, err := sessionMgr.UserSession(context.TODO())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup user session: %v", err)
		}

		restClient := rest.NewClient(client)
		restClient.Login(context.TODO(), s.URL.User)
		vcs[vCenterName] = &VCenter{
			VMClient:   client,
			TagManager: vapitags.NewManager(restClient),
			Username:   userSession.UserName,
			Model:      model,
		}

		sessions = append(sessions, s)
		models = append(models, &model)
	}

	clusterInfo := util.NewClusterInfo()

	testSetup := &TestSetup{
		Context:     context.TODO(),
		VMConfig:    GetVSphereConfig(cloudConfig),
		ClusterInfo: clusterInfo,
		VCenters:    vcs,
	}

	// Default config uses legacy config.  We can set this, but will check to make sure not missing
	/*if testSetup.VMConfig.LegacyConfig != nil {
		testSetup.VMConfig.LegacyConfig.Workspace.VCenterIP = "dc0"
		testSetup.VMConfig.LegacyConfig.VirtualCenter["dc0"].User = userSession.UserName
	}*/

	cleanup = func() {
		for s := range sessions {
			sessions[s].Close()
		}
		for model := range models {
			models[model].Remove()
		}
	}
	return testSetup, cleanup, nil
}

type FakeKubeClient struct {
	Infrastructure *ocpv1.Infrastructure
	Nodes          []*v1.Node
	StorageClasses []*storagev1.StorageClass
	PVs            []*v1.PersistentVolume
}

func (f *FakeKubeClient) GetInfrastructure(ctx context.Context) (*ocpv1.Infrastructure, error) {
	return f.Infrastructure, nil
}

func (f *FakeKubeClient) ListNodes(ctx context.Context) ([]*v1.Node, error) {
	return f.Nodes, nil
}

func (f *FakeKubeClient) ListStorageClasses(ctx context.Context) ([]*storagev1.StorageClass, error) {
	return f.StorageClasses, nil
}

func (f *FakeKubeClient) ListPVs(ctx context.Context) ([]*v1.PersistentVolume, error) {
	return f.PVs, nil
}

func Node(name string, modifiers ...func(*v1.Node)) *v1.Node {
	n := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.NodeSpec{
			ProviderID: "",
		},
	}
	for _, modifier := range modifiers {
		modifier(n)
	}
	return n
}

func WithProviderID(id string) func(*v1.Node) {
	return func(node *v1.Node) {
		node.Spec.ProviderID = id
	}
}

func DefaultNodes() []*v1.Node {
	nodes := []*v1.Node{}
	for _, vm := range defaultVMs {
		node := Node(vm.Name, WithProviderID("vsphere://"+vm.UUID))
		nodes = append(nodes, node)
	}
	return nodes
}

func Infrastructure(modifiers ...func(*ocpv1.Infrastructure)) *ocpv1.Infrastructure {
	infra := &ocpv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocpv1.InfrastructureSpec{
			CloudConfig: ocpv1.ConfigMapFileReference{
				Key:  "config",
				Name: "cloud-provider-config",
			},
			PlatformSpec: ocpv1.PlatformSpec{
				VSphere: &ocpv1.VSpherePlatformSpec{},
			},
		},
		Status: ocpv1.InfrastructureStatus{
			InfrastructureName: "my-cluster-id",
		},
	}

	for _, modifier := range modifiers {
		modifier(infra)
	}
	return infra
}

func InfrastructureWithFailureDomain(modifiers ...func(*ocpv1.Infrastructure)) *ocpv1.Infrastructure {
	infra := Infrastructure()

	// Add failure domain
	var fds []ocpv1.VSpherePlatformFailureDomainSpec
	fds = append(fds, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "dc0",
		Region: "east",
		Server: "dc0",
		Topology: ocpv1.VSpherePlatformTopology{
			ComputeCluster: "/DC0/host/DC0_C0",
			Datacenter:     "DC0",
			Datastore:      "LocalDS_0",
		},
		Zone: "east-1a",
	})

	infra.Spec.PlatformSpec.VSphere.FailureDomains = fds

	// Add vCenters to vCenter section
	infra.Spec.PlatformSpec.VSphere.VCenters = []ocpv1.VSpherePlatformVCenterSpec{
		{
			Server: "dc0",
			Datacenters: []string{
				"DC0",
			},
		},
	}

	for _, modifier := range modifiers {
		modifier(infra)
	}
	return infra
}

func InfrastructureWithMultipleFailureDomain(modifiers ...func(*ocpv1.Infrastructure)) *ocpv1.Infrastructure {
	infra := InfrastructureWithFailureDomain()

	// Add failure domain
	fds := infra.Spec.PlatformSpec.VSphere.FailureDomains
	fds = append(fds, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "dc0",
		Region: "west",
		Server: "dc0",
		Topology: ocpv1.VSpherePlatformTopology{
			ComputeCluster: "/DC1/host/DC0_C1",
			Datacenter:     "DC1",
			Datastore:      "LocalDS_1",
		},
		Zone: "west-1a",
	})

	infra.Spec.PlatformSpec.VSphere.FailureDomains = fds

	for _, modifier := range modifiers {
		modifier(infra)
	}
	return infra
}

func InfrastructureWithMultipleFailureDomainHostGroup(modifiers ...func(*ocpv1.Infrastructure)) *ocpv1.Infrastructure {
	infra := InfrastructureWithFailureDomain()

	// Add failure domain
	fds := infra.Spec.PlatformSpec.VSphere.FailureDomains
	fds = append(fds, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "dc0",
		Region: "west",
		Server: "dc0",
		Topology: ocpv1.VSpherePlatformTopology{
			ComputeCluster: "/DC0/host/DC0_C1",
			Datacenter:     "DC0",
			Datastore:      "LocalDS_0",
			Folder:         "/DC0/vm",
			Networks: []string{
				"DC0_DVPG0",
			},
			ResourcePool: "/DC0/host/DC0_C1/Resources/test-resourcepool",
		},
		Zone: "west-1a",
		ZoneAffinity: &ocpv1.VSphereFailureDomainZoneAffinity{
			Type: ocpv1.HostGroupFailureDomainZone,
			HostGroup: &ocpv1.VSphereFailureDomainHostGroup{
				VMGroup:    "vm-zone-1",
				HostGroup:  "zone-1",
				VMHostRule: "vm-rule-zone-1",
			},
		},
	})

	infra.Spec.PlatformSpec.VSphere.FailureDomains = fds

	for _, modifier := range modifiers {
		modifier(infra)
	}
	return infra
}

func InfrastructureWithMultiVCenters(modifiers ...func(*ocpv1.Infrastructure)) *ocpv1.Infrastructure {
	infra := Infrastructure()

	// Add failure domain.
	var fds []ocpv1.VSpherePlatformFailureDomainSpec

	// Create FD for vCenter #1
	fds = append(fds, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "failure-domain-1",
		Region: "east",
		Server: "vcenter.test.openshift.com",
		Topology: ocpv1.VSpherePlatformTopology{
			Datacenter: "DC0",
			Datastore:  "LocalDS_0",
		},
		Zone: "east-1a",
	})

	// Create FD for vCenter #2
	fds = append(fds, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "failure-domain-2",
		Region: "west",
		Server: "vcenter2.test.openshift.com",
		Topology: ocpv1.VSpherePlatformTopology{
			Datacenter: "DC1",
			Datastore:  "LocalDS_0",
		},
		Zone: "west-1a",
	})

	// Add failure domains to spec
	infra.Spec.PlatformSpec.VSphere.FailureDomains = fds

	// Add vCenters to vCenter section
	vcs := []ocpv1.VSpherePlatformVCenterSpec{
		{
			Server: "vcenter.test.openshift.com",
			Datacenters: []string{
				"DC0",
			},
		},
		{
			Server: "vcenter2.test.openshift.com",
			Datacenters: []string{
				"DC1",
			},
		},
	}

	infra.Spec.PlatformSpec.VSphere.VCenters = vcs

	for _, modifier := range modifiers {
		modifier(infra)
	}
	return infra
}

func GetVM(vmClient *vim25.Client, node *v1.Node) (*mo.VirtualMachine, error) {
	finder := find.NewFinder(vmClient, true)
	vm, err := finder.VirtualMachine(context.TODO(), defaultVMPath+node.Name)
	if err != nil {
		return nil, err
	}

	var o mo.VirtualMachine
	err = vm.Properties(context.TODO(), vm.Reference(), NodeProperties, &o)
	if err != nil {
		return nil, fmt.Errorf("failed to load VM %s: %s", node.Name, err)
	}

	return &o, nil
}

func CustomizeVM(vmClient *vim25.Client, node *v1.Node, spec *types.VirtualMachineConfigSpec) error {
	finder := find.NewFinder(vmClient, true)
	vm, err := finder.VirtualMachine(context.TODO(), defaultVMPath+node.Name)
	if err != nil {
		return err
	}

	task, err := vm.Reconfigure(context.TODO(), *spec)
	if err != nil {
		return err
	}

	err = task.Wait(context.TODO())
	return err
}

func SetHardwareVersion(vmClient *vim25.Client, node *v1.Node, hardwareVersion string) error {
	err := CustomizeVM(vmClient, node, &types.VirtualMachineConfigSpec{
		ExtraConfig: []types.BaseOptionValue{
			&types.OptionValue{
				Key: "SET.config.version", Value: hardwareVersion,
			},
		}})
	return err
}

func CustomizeHostVersion(hostSystemId string, version string, apiVersion string) error {
	hsRef := simulator.Map.Get(types.ManagedObjectReference{
		Type:  "HostSystem",
		Value: hostSystemId,
	})
	if hsRef == nil {
		return fmt.Errorf("can't find HostSystem %s", hostSystemId)
	}

	hs := hsRef.(*simulator.HostSystem)
	hs.Config.Product.Version = version
	hs.Config.Product.ApiVersion = apiVersion
	return nil
}
