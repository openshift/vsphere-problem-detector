package check

import (
	"fmt"
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
	"gopkg.in/gcfg.v1"

	ocpv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/legacy-cloud-providers/vsphere"
)

func SetupSimulator(kubeClient *testlib.FakeKubeClient, modelDir string) (ctx *CheckContext, cleanup func(), err error) {
	setup, cleanup, err := testlib.SetupSimulator(kubeClient, modelDir)

	ctx = &CheckContext{
		Context:     setup.Context,
		VMConfig:    setup.VMConfig,
		TagManager:  setup.TagManager,
		ClusterInfo: setup.ClusterInfo,
		VMClient:    setup.VMClient,
		Cache:       cache.NewCheckCache(setup.VMClient),
		KubeClient:  kubeClient,
	}

	ctx.Username = setup.Username
	if kubeClient != nil && kubeClient.Infrastructure != nil {
		ConvertToPlatformSpec(kubeClient.Infrastructure, ctx)
	}
	return ctx, cleanup, err
}

func TestDatastoreByURL(t *testing.T) {
	tests := []struct {
		name         string
		infra        *ocpv1.Infrastructure
		dataStoreURL string
		expectError  bool
	}{
		{
			name:         "with single failure-domain and valid datastore url",
			infra:        testlib.Infrastructure(),
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_3-206027153",
			expectError:  false,
		},
		{
			name:         "with single failure-domain and not-valid datastore url",
			dataStoreURL: "testdata/default/govcsim-DC0-LocalDS_100-206027153",
			infra:        testlib.Infrastructure(),
			expectError:  true,
		},
		{
			name: "with multiple failure-domain and only one having valid datastore url",
			infra: testlib.Infrastructure(func(infra *ocpv1.Infrastructure) {
				infra.Spec.PlatformSpec.VSphere.FailureDomains = append(infra.Spec.PlatformSpec.VSphere.FailureDomains, ocpv1.VSpherePlatformFailureDomainSpec{
					Name: "other one",
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
			infra: testlib.Infrastructure(func(infra *ocpv1.Infrastructure) {
				infra.Spec.PlatformSpec.VSphere.FailureDomains = append(infra.Spec.PlatformSpec.VSphere.FailureDomains, ocpv1.VSpherePlatformFailureDomainSpec{
					Name: "other one",
					Topology: ocpv1.VSpherePlatformTopology{
						Datacenter: "DC1",
						Datastore:  "local-ds1000",
					},
				})
			}),
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
			ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
			if err != nil {
				t.Fatalf("unexpected error setting up simulator: %v", err)
			}
			defer cleanup()

			_, _, err = getDatastoreByURL(ctx, test.dataStoreURL)
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
	config := prolematicConfig(t)
	infra := getProblemInfra()

	kubeClient := &testlib.FakeKubeClient{
		Infrastructure: infra,
		Nodes:          testlib.DefaultNodes(),
	}
	setup, cleanup, err := testlib.SetupSimulator(kubeClient, testlib.DefaultModel)
	if err != nil {
		t.Errorf("Error setting up simulator: %v", err)
	}
	defer cleanup()

	ctx := &CheckContext{
		Context:     setup.Context,
		VMConfig:    config,
		TagManager:  setup.TagManager,
		ClusterInfo: setup.ClusterInfo,
		VMClient:    setup.VMClient,
		Cache:       cache.NewCheckCache(setup.VMClient),
		KubeClient:  kubeClient,
	}

	ctx.Username = setup.Username

	ConvertToPlatformSpec(infra, ctx)
	if len(ctx.PlatformSpec.FailureDomains) == 0 {
		t.Errorf("FailureDomains should not be empty")
	}

	failureDomain := ctx.PlatformSpec.FailureDomains[0]
	topology := failureDomain.Topology
	if topology.Datacenter != config.Workspace.Datacenter {
		t.Errorf("Datacenter in topology should be %s, got %s", config.Workspace.Datacenter, topology.Datacenter)
	}

	if topology.Datastore != config.Workspace.DefaultDatastore {
		t.Errorf("Datastore in topology should be %s, got %s", config.Workspace.DefaultDatastore, topology.Datastore)
	}
}

func prolematicConfig(t *testing.T) *vsphere.VSphereConfig {
	data := `
	[Global]
	secret-name=vsphere-creds
	secret-namespace=kube-system
	insecure-flag=1


    [Workspace]
    server=foobar.com
    datacenter=dc443
    default-datastore=/foobar/datastore
    folder=/foobar/folder

    [VirtualCenter "foobar.com"]
    datacenters=dc443
	`
	var cfg vsphere.VSphereConfig
	err := gcfg.ReadStringInto(&cfg, data)
	if err != nil {
		t.Errorf("Error reading config: %v", err)
		return nil
	}
	return &cfg
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
