package check

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"testing"

	ocpv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

func SetupSimulator(kubeClient *testlib.FakeKubeClient, modelDir string) (ctx *CheckContext, cleanup func(), err error) {
	return SetupSimulatorWithConfig(kubeClient, modelDir, "")
}

func SetupSimulatorWithConfig(kubeClient *testlib.FakeKubeClient, modelDir, configFileName string) (ctx *CheckContext, cleanup func(), err error) {
	setup, cleanup, err := testlib.SetupSimulatorWithConfig(kubeClient, modelDir, configFileName)

	vcMap := make(map[string]*VCenter)

	for vCenterName := range setup.VMConfig.Config.VirtualCenter {
		vc := VCenter{
			VCenterName: vCenterName,
			TagManager:  setup.VCenters[vCenterName].TagManager,
			VMClient:    setup.VCenters[vCenterName].VMClient,
			Cache:       cache.NewCheckCache(setup.VCenters[vCenterName].VMClient),
			Username:    setup.VCenters[vCenterName].Username,
		}
		vcMap[vCenterName] = &vc
	}

	ctx = &CheckContext{
		Context:     setup.Context,
		VMConfig:    setup.VMConfig,
		ClusterInfo: setup.ClusterInfo,
		KubeClient:  kubeClient,
		VCenters:    vcMap,
	}

	if kubeClient != nil && kubeClient.Infrastructure != nil {
		ConvertToPlatformSpec(kubeClient.Infrastructure, ctx)
	}
	return ctx, cleanup, err
}

func TestDatastoreByURL(t *testing.T) {
	tests := []struct {
		name         string
		cloudConfig  string
		infra        *ocpv1.Infrastructure
		dataStoreURL string
		expectError  bool
	}{
		{
			name:         "with no failure-domain but with valid datastore url",
			infra:        testlib.Infrastructure(),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_3-206027153",
			expectError:  false,
		},
		{
			name:         "with no failure-domain defined but with not-valid datastore url",
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_100-206027153",
			infra:        testlib.Infrastructure(),
			expectError:  true,
		},
		{
			name:         "with single failure-domain and valid datastore url",
			infra:        testlib.InfrastructureWithFailureDomain(),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_3-206027153",
			expectError:  false,
		},
		{
			name:         "with single failure-domain and not-valid datastore url",
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_100-206027153",
			infra:        testlib.InfrastructureWithFailureDomain(),
			expectError:  true,
		},
		{
			name: "with multiple failure-domain and only one having valid datastore url",
			infra: testlib.InfrastructureWithFailureDomain(func(infra *ocpv1.Infrastructure) {
				infra.Spec.PlatformSpec.VSphere.FailureDomains = append(infra.Spec.PlatformSpec.VSphere.FailureDomains, ocpv1.VSpherePlatformFailureDomainSpec{
					Name:   "other one",
					Server: "dc0",
					Topology: ocpv1.VSpherePlatformTopology{
						Datacenter: "DC1",
						Datastore:  "local-ds1000",
					},
				})
			}),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_3-206027153",
			expectError:  false,
		},
		{
			name: "with multiple failure-domain and none having valid datastore",
			infra: testlib.InfrastructureWithFailureDomain(func(infra *ocpv1.Infrastructure) {
				infra.Spec.PlatformSpec.VSphere.FailureDomains = append(infra.Spec.PlatformSpec.VSphere.FailureDomains, ocpv1.VSpherePlatformFailureDomainSpec{
					Name:   "other one",
					Server: "dc0",
					Topology: ocpv1.VSpherePlatformTopology{
						Datacenter: "DC1",
						Datastore:  "local-ds1000",
					},
				})
			}),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_100-206027153",
			expectError:  true,
		},
		{
			name:         "with multiple vCenters and first having valid datastore url",
			cloudConfig:  "simple_config.yaml",
			infra:        testlib.InfrastructureWithMultiVCenters(),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_3-206027153",
			expectError:  false,
		},
		{
			name:         "with multiple vCenters and second having valid datastore url",
			cloudConfig:  "simple_config.yaml",
			infra:        testlib.InfrastructureWithMultiVCenters(),
			dataStoreURL: "testdata/default/govcsim-DC1-LocalDS_1-057538539",
			expectError:  false,
		},
		{
			name:         "with multiple vCenters and none having valid datastore",
			cloudConfig:  "simple_config.yaml",
			infra:        testlib.InfrastructureWithMultiVCenters(),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_100-206027153",
			expectError:  true,
		},
	}

	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			kubeClient := &testlib.FakeKubeClient{
				Infrastructure: test.infra,
				Nodes:          testlib.DefaultNodes(),
			}
			ctx, cleanup, err := SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, test.cloudConfig)
			if err != nil {
				t.Fatalf("unexpected error setting up simulator: %v", err)
			}
			defer cleanup()

			_, _, _, err = getDatastoreByURL(ctx, test.dataStoreURL)
			if !test.expectError && err != nil {
				t.Errorf("unexpected error finding datastore: %v", err)
			}

			if test.expectError && err == nil {
				t.Errorf("expected error while finding datastore, found none")
			}
		})
	}
}

func TestMissingFailureDomains(t *testing.T) {
	ocpv1.Install(scheme.Scheme)
	infra := getProblemInfra()

	kubeClient := &testlib.FakeKubeClient{
		Infrastructure: infra,
		Nodes:          testlib.DefaultNodes(),
	}
	ctx, cleanup, err := SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, "problematic_config.ini")
	if err != nil {
		t.Errorf("Error setting up simulator: %v", err)
	}
	defer cleanup()

	if len(ctx.PlatformSpec.FailureDomains) == 0 {
		t.Errorf("FailureDomains should not be empty")
	}

	failureDomain := ctx.PlatformSpec.FailureDomains[0]
	topology := failureDomain.Topology
	config := ctx.VMConfig
	if topology.Datacenter != config.LegacyConfig.Workspace.Datacenter {
		t.Errorf("Datacenter in topology should be %s, got %s", config.LegacyConfig.Workspace.Datacenter, topology.Datacenter)
	}

	if topology.Datastore != config.LegacyConfig.Workspace.DefaultDatastore {
		t.Errorf("Datastore in topology should be %s, got %s", config.LegacyConfig.Workspace.DefaultDatastore, topology.Datastore)
	}
}

// Test to perform:
// - No platform spec
// - platform spec with 1 FD
// - platform spec w/ multi FD
// - platform spec with multi vCenter
func TestGetVCenter(t *testing.T) {
	tests := []struct {
		name        string
		cloudConfig string
		infra       *ocpv1.Infrastructure
		node        *v1.Node
		server      string
		expectErr   string
	}{
		{
			name:        "no platform spec",
			cloudConfig: "simple_config.ini",
			infra:       testlib.Infrastructure(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				return node
			}(),
			server: "dc0",
		},
		{
			name:        "platform spec with one failure domain (ini)",
			cloudConfig: "simple_config.ini",
			infra:       testlib.InfrastructureWithFailureDomain(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				return node
			}(),
			server: "dc0",
		},
		{
			name:        "platform spec with one failure domain (yaml)",
			cloudConfig: "config_single-vcenter.yaml",
			infra: func() *ocpv1.Infrastructure {
				infra := testlib.InfrastructureWithMultiVCenters()
				infra.Spec.PlatformSpec.VSphere.FailureDomains = infra.Spec.PlatformSpec.VSphere.FailureDomains[0:1]
				infra.Spec.PlatformSpec.VSphere.VCenters = infra.Spec.PlatformSpec.VSphere.VCenters[0:1]
				return infra
			}(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				return node
			}(),
			server: "vcenter.test.openshift.com",
		},
		{
			name:        "platform spec with multiple failure domains (ini)",
			cloudConfig: "simple_config.ini",
			infra:       testlib.InfrastructureWithMultipleFailureDomain(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelTopologyRegion] = "west"
				node.Labels[v1.LabelTopologyZone] = "west-1a"
				return node
			}(),
			server: "dc0",
		},
		{
			name:        "platform spec with multiple failure domains (yaml)",
			cloudConfig: "config_single-vcenter.yaml",
			infra: func() *ocpv1.Infrastructure {
				infra := testlib.InfrastructureWithMultipleFailureDomain()
				for index := range infra.Spec.PlatformSpec.VSphere.FailureDomains {
					infra.Spec.PlatformSpec.VSphere.FailureDomains[index].Server = "vcenter.test.openshift.com"
				}
				infra.Spec.PlatformSpec.VSphere.VCenters = infra.Spec.PlatformSpec.VSphere.VCenters[0:1]
				infra.Spec.PlatformSpec.VSphere.VCenters[0].Server = "vcenter.test.openshift.com"
				return infra
			}(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelTopologyRegion] = "west"
				node.Labels[v1.LabelTopologyZone] = "west-1a"
				return node
			}(),
			server: "vcenter.test.openshift.com",
		},
		{
			name:        "platform spec with multiple failure domains beta labels (ini)",
			cloudConfig: "simple_config.ini",
			infra:       testlib.InfrastructureWithMultipleFailureDomain(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelFailureDomainBetaRegion] = "east"
				node.Labels[v1.LabelFailureDomainBetaZone] = "east-1a"
				return node
			}(),
			server: "dc0",
		},
		{
			name:        "platform spec with multiple failure domains beta labels (yaml)",
			cloudConfig: "config_single-vcenter.yaml",
			infra: func() *ocpv1.Infrastructure {
				infra := testlib.InfrastructureWithMultipleFailureDomain()
				for index := range infra.Spec.PlatformSpec.VSphere.FailureDomains {
					infra.Spec.PlatformSpec.VSphere.FailureDomains[index].Server = "vcenter.test.openshift.com"
				}
				infra.Spec.PlatformSpec.VSphere.VCenters = infra.Spec.PlatformSpec.VSphere.VCenters[0:1]
				infra.Spec.PlatformSpec.VSphere.VCenters[0].Server = "vcenter.test.openshift.com"
				return infra
			}(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelFailureDomainBetaRegion] = "east"
				node.Labels[v1.LabelFailureDomainBetaZone] = "east-1a"
				return node
			}(),
			server: "vcenter.test.openshift.com",
		},
		{
			name:        "platform spec with multiple failure domains unknown fd (ini)",
			cloudConfig: "simple_config.ini",
			infra:       testlib.InfrastructureWithMultipleFailureDomain(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelTopologyRegion] = "foo"
				node.Labels[v1.LabelTopologyZone] = "bar"
				return node
			}(),
			expectErr: "unable to determine vcenter for node DC0_H0_VM0",
		},
		{
			name:        "platform spec with multiple failure domains unknown fd (yaml)",
			cloudConfig: "config_single-vcenter.yaml",
			infra: func() *ocpv1.Infrastructure {
				infra := testlib.InfrastructureWithMultipleFailureDomain()
				for index := range infra.Spec.PlatformSpec.VSphere.FailureDomains {
					infra.Spec.PlatformSpec.VSphere.FailureDomains[index].Server = "vcenter.test.openshift.com"
				}
				infra.Spec.PlatformSpec.VSphere.VCenters = infra.Spec.PlatformSpec.VSphere.VCenters[0:1]
				infra.Spec.PlatformSpec.VSphere.VCenters[0].Server = "vcenter.test.openshift.com"
				return infra
			}(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelTopologyRegion] = "foo"
				node.Labels[v1.LabelTopologyZone] = "bar"
				return node
			}(),
			expectErr: "unable to determine vcenter for node DC0_H0_VM0",
		},
		{
			name:        "platform spec with multiple failure domains unknown fd beta labels (ini)",
			cloudConfig: "simple_config.ini",
			infra:       testlib.InfrastructureWithMultipleFailureDomain(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelFailureDomainBetaRegion] = "foo"
				node.Labels[v1.LabelFailureDomainBetaZone] = "bar"
				return node
			}(),
			expectErr: "unable to determine vcenter for node DC0_H0_VM0",
		},
		{
			name:        "platform spec with multiple failure domains no labels",
			cloudConfig: "simple_config.ini",
			infra:       testlib.InfrastructureWithFailureDomain(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				return node
			}(),
			server: "dc0",
		},
		{
			name:        "platform spec with multiple vcenters",
			cloudConfig: "simple_config.yaml",
			infra:       testlib.InfrastructureWithMultiVCenters(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelFailureDomainBetaRegion] = "east"
				node.Labels[v1.LabelFailureDomainBetaZone] = "east-1a"
				return node
			}(),
			server: "vcenter.test.openshift.com",
		},
		{
			name:        "platform spec with multiple vcenters unknown fd",
			cloudConfig: "simple_config.yaml",
			infra:       testlib.InfrastructureWithMultiVCenters(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				node.Labels = make(map[string]string)
				node.Labels[v1.LabelFailureDomainBetaRegion] = "foo"
				node.Labels[v1.LabelFailureDomainBetaZone] = "bar"
				return node
			}(),
			expectErr: "unable to determine vcenter for node DC0_H0_VM0",
		},
		{
			name:        "platform spec with multiple vcenters no labels",
			cloudConfig: "simple_config.yaml",
			infra:       testlib.InfrastructureWithMultiVCenters(),
			node: func() *v1.Node {
				node := testlib.DefaultNodes()[0]
				return node
			}(),
			server: "vcenter.test.openshift.com",
		},
	}

	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			kubeClient := &testlib.FakeKubeClient{
				Infrastructure: test.infra,
				Nodes:          testlib.DefaultNodes(),
			}
			ctx, cleanup, err := SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, test.cloudConfig)
			if err != nil {
				t.Fatalf("unexpected error setting up simulator: %v", err)
			}
			defer cleanup()

			vCenter, err := GetVCenter(ctx, test.node)

			checkPassed := true
			if test.expectErr == "" {
				checkPassed = assert.NoError(t, err)
			} else {
				checkPassed = assert.Regexp(t, test.expectErr, err)
			}

			if !checkPassed {
				return
			}

			if len(test.server) > 0 {
				if vCenter == nil {
					assert.Fail(t, "no vCenter was returned during test")
				} else if vCenter.VCenterName != test.server {
					assert.Fail(t, fmt.Sprintf("result vCenter should be %s, not %s", test.server, vCenter.VCenterName))
				}
			}

		})
	}
}

func convertYAMLToObject(yamlString string) (runtime.Object, error) {
	// Decode the YAML string into a JSON object
	jsonBytes, err := yaml.ToJSON([]byte(yamlString))
	if err != nil {
		return nil, err
	}

	// Create a new decoder
	decoder := scheme.Codecs.UniversalDeserializer()

	// Decode the JSON object into a runtime.Object
	obj, _, err := decoder.Decode(jsonBytes, nil, nil)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func getProblemInfra() *ocpv1.Infrastructure {
	infraString := `
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  cloudConfig:
    key: config
    name: cloud-provider-config
  platformSpec:
    type: VSphere
    vsphere:
      nodeNetworking:
        external: {}
        internal: {}
      vcenters:
      - datacenters:
        - dc443
        port: 443
        server: foobar.com
status:
  apiServerInternalURI: foobar.com:6443
  apiServerURL: https://foobar.com:6443
  controlPlaneTopology: HighlyAvailable
  cpuPartitioning: None
  etcdDiscoveryDomain: ''
  infrastructureName: emacs-1234
  infrastructureTopology: HighlyAvailable
  platform: VSphere
  platformStatus:
    type: VSphere`

	obj, err := convertYAMLToObject(infraString)
	if err != nil {
		fmt.Println("Error converting YAML to object: ", err)
		return nil
	}

	// Use the converted object
	infra := obj.(*ocpv1.Infrastructure)
	return infra
	// Do something with the infra object
}
