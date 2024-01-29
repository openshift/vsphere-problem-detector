package check

import (
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"

	ocpv1 "github.com/openshift/api/config/v1"
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
