package check

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	datastoreTests = []struct {
		name        string
		datastore   string
		expectError bool
	}{
		{
			name:        "short datastore",
			datastore:   "LocalDS_1",
			expectError: false,
		},
		{
			name:        "non-existant datastore",
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
		},
		{
			name:        "datastore which is not part of a datastore cluster",
			datastore:   "/DC0/datastore/LocalDS_1",
			expectError: false,
		},
	}
)

func TestCheckDefaultDatastore(t *testing.T) {
	for _, test := range datastoreTests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			kubeClient := &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes:          defaultNodes(),
			}
			ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

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
	for _, test := range datastoreTests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			kubeClient := &fakeKubeClient{
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
			ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Act
			err = CheckStorageClasses(ctx)

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

func TestCheckPVs(t *testing.T) {
	var (
		pvWithDatastoreNames = []struct {
			name        string
			datastore   string
			expectError bool
		}{
			{
				name:        "short datastore",
				datastore:   "LocalDS_1",
				expectError: false,
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
		}
	)

	for _, test := range pvWithDatastoreNames {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			kubeClient := &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes:          defaultNodes(),
				pvs: []*v1.PersistentVolume{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: test.name,
						},
						Spec: v1.PersistentVolumeSpec{
							PersistentVolumeSource: v1.PersistentVolumeSource{
								VsphereVolume: &v1.VsphereVirtualDiskVolumeSource{
									VolumePath: fmt.Sprintf("[%s] 00000000-0000-0000-0000-000000000000/my-cluster-id-dynamic-pvc-00000000-0000-0000-0000-000000000000.vmdk", test.datastore),
								},
							},
						},
					},
				},
			}
			ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Act
			err = CheckPVs(ctx)

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
