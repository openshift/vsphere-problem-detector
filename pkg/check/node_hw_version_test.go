package check

import (
	"strings"
	"testing"

	testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/component-base/metrics/legacyregistry"
)

func TestCollectNodeHWVersion(t *testing.T) {
	tests := []struct {
		name            string
		hwVersion       string
		expectedMetrics string
	}{
		{
			name:      "hw ver 13",
			hwVersion: "vmx-13",
			// There are two VMs. The first one gets hwVersion from the test, the seconds one keeps the default (vmx-13)
			expectedMetrics: `
# HELP vsphere_node_hw_version_total [ALPHA] Number of vSphere nodes with given HW version.
# TYPE vsphere_node_hw_version_total gauge
vsphere_node_hw_version_total{hw_version="vmx-13"} 2
`,
		},
		{
			name:      "hw ver 15",
			hwVersion: "vmx-15",
			expectedMetrics: `
# HELP vsphere_node_hw_version_total [ALPHA] Number of vSphere nodes with given HW version.
# TYPE vsphere_node_hw_version_total gauge
vsphere_node_hw_version_total{hw_version="vmx-13"} 1
vsphere_node_hw_version_total{hw_version="vmx-15"} 1
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			check := CollectNodeHWVersion{}
			kubeClient := &fakeKubeClient{
				nodes: defaultNodes(),
			}
			ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Set HW version of the first VM. Leave the other VMs with the default version (vmx-13).
			node := &kubeClient.nodes[0]
			err = customizeVM(ctx, node, &types.VirtualMachineConfigSpec{
				ExtraConfig: []types.BaseOptionValue{
					&types.OptionValue{
						Key: "SET.config.version", Value: test.hwVersion,
					},
				}})
			if err != nil {
				t.Fatalf("Failed to customize node: %s", err)
			}

			// Reset metrics from previous tests. Note: the tests can't run in parallel!
			legacyregistry.Reset()

			// Act - simulate loop through all nodes
			err = check.StartCheck()
			if err != nil {
				t.Errorf("StartCheck failed: %s", err)
			}

			for _, node := range kubeClient.nodes {
				vm, err := getVM(ctx, &node)
				if err != nil {
					t.Errorf("Error getting vm for node %s: %s", node.Name, err)
				}
				err = check.CheckNode(ctx, &node, vm)
				if err != nil {
					t.Errorf("Unexpected error on node %s: %s", node.Name, err)
				}
			}

			check.FinishCheck(ctx)

			// Assert
			if err := testutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(test.expectedMetrics), "vsphere_node_hw_version_total"); err != nil {
				t.Errorf("Unexpected metric: %s", err)
			}
		})
	}
}
