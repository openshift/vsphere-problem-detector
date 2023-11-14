package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	testutil "github.com/prometheus/client_golang/prometheus/testutil"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/metrics/legacyregistry"
)

var (
	datastoreTests = []struct {
		name        string
		datastore   string
		expectError bool
		dsType      string
	}{
		{
			name:        "short datastore",
			datastore:   "LocalDS_1",
			expectError: false,
			dsType:      "OTHER",
		},
		{
			name:        "non-existing datastore",
			datastore:   "foobar", // this datastore does not exist and hence should result in error
			expectError: true,
		},
		{
			name:        "long datastore",
			datastore:   "01234567890123456789012345678901234567890123456789", // 269 characters in the escaped path
			expectError: true,
		},
		{
			name:        "short datastore with too many dashes",
			datastore:   "0-1-2-3-4-5-6-7-8-9", // 265 characters in the escaped path
			expectError: true,
		},
		{
			name:        "datastore which is part of a datastore cluster",
			datastore:   "/DC0/datastore/DC0_POD0/LocalDS_2",
			expectError: true,
			dsType:      "OTHER",
		},
		{
			name:        "datastore which is not part of a datastore cluster",
			datastore:   "/DC0/datastore/LocalDS_1",
			expectError: false,
			dsType:      "OTHER",
		},
	}
)

func TestCheckDefaultDatastore(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	for _, test := range datastoreTests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			dataStoreTypesMetric.Reset()
			kubeClient := &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes:          defaultNodes(),
			}
			ctx, cleanup, err := SetupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			authManager, err := getAuthManagerWithValidPrivileges(ctx, mockCtrl)
			if err != nil {
				t.Fatalf("authManager setup failed: %s", err)
			}
			ctx.AuthManager = authManager

			ctx.VMConfig.Workspace.DefaultDatastore = test.datastore
			// Act
			err = CheckDefaultDatastore(ctx)

			// Assert
			if err != nil && !test.expectError {
				t.Errorf("Unexpected error: %s", err)
			}
			if err == nil && test.expectError {
				t.Errorf("Expected error, got none")
			}
		})
	}
}

func TestCheckStorageClassesWithDatastore(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	for _, test := range datastoreTests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			kubeClient := &testlib.FakeKubeClient{
				infrastructure: infrastructure(),
				nodes:          defaultNodes(),
				storageClasses: []*storagev1.StorageClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: test.name,
						},
						Provisioner: "kubernetes.io/vsphere-volume",
						Parameters: map[string]string{
							// TODO: add tests with storagePolicyName
							"datastore": test.datastore,
						},
					},
				},
			}
			ctx, cleanup, err := SetupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			authManager, err := getAuthManagerWithValidPrivileges(ctx, mockCtrl)
			if err != nil {
				t.Fatalf("authManager setup failed: %s", err)
			}
			ctx.AuthManager = authManager

			// Reset metrics from previous tests. Note: the tests can't run in parallel!
			legacyregistry.Reset()

			// Act
			err = CheckStorageClasses(ctx)

			// Assert
			if err != nil && !test.expectError {
				t.Errorf("Unexpected error: %s", err)
			}
			if err == nil && test.expectError {
				t.Errorf("Expected error, got none")
			}
			if test.dsType != "" {

				// UUID & version is hardcoded in the simulator, github.com/vmware/govmomi/simulator/vpx/service_content.go
				expectedMetricFmt := `
					# HELP vsphere_datastore_total [ALPHA] Number of DataStores used by the cluster.
					# TYPE vsphere_datastore_total gauge
					vsphere_datastore_total{type="%s"} 1
`
				expectedMetric := fmt.Sprintf(expectedMetricFmt, strings.ToLower(test.dsType))

				if err := testutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(expectedMetric), "vsphere_datastore_total"); err != nil {
					t.Errorf("Unexpected metric: %s", err)
				}
			}
		})
	}
}
