package check

import (
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
	"github.com/vmware/govmomi/vim25/types"
)

func TestCheckNodeDiskUUID(t *testing.T) {
	tests := []struct {
		name        string
		uuidEnabled bool
		expectError bool
	}{
		{
			name:        "enabled true",
			uuidEnabled: true,
			expectError: false,
		},
		{
			name:        "enabled false",
			uuidEnabled: false,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			check := CheckNodeDiskUUID{}
			err := check.StartCheck()
			if err != nil {
				t.Errorf("StartCheck failed: %s", err)
			}

			kubeClient := &testlib.FakeKubeClient{
				Nodes: testlib.DefaultNodes(),
			}
			ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Set VM disk.enableUUID
			node := kubeClient.Nodes[0]
			err = testlib.CustomizeVM(ctx.VMClient, node, &types.VirtualMachineConfigSpec{
				ExtraConfig: []types.BaseOptionValue{
					&types.OptionValue{
						Key: "SET.config.flags.diskUuidEnabled", Value: test.uuidEnabled,
					},
				}})
			if err != nil {
				t.Fatalf("Failed to customize node: %s", err)
			}

			vm, err := testlib.GetVM(ctx.VMClient, node)
			if err != nil {
				t.Errorf("Error getting vm for node %s: %s", node.Name, err)
			}

			// Act
			err = check.CheckNode(ctx, kubeClient.Nodes[0], vm)

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
