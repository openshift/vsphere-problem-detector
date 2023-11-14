package check

import (
	"strings"
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/metrics"
	testutil "github.com/prometheus/client_golang/prometheus/testutil"
	basemetrics "k8s.io/component-base/metrics"
)

func TestCollectNodeHWVersion(t *testing.T) {
	tests := []struct {
		name            string
		hwVersions      []string
		expectedMetrics string
	}{
		{
			name: "hw ver 13",
			// There are two VMs. The first one gets hwVersion from the test, the seconds one keeps the default (vmx-13)
			expectedMetrics: `
# HELP vsphere_node_hw_version_total [ALPHA] Number of vSphere nodes with given HW version.
# TYPE vsphere_node_hw_version_total gauge
vsphere_node_hw_version_total{hw_version="vmx-13"} 2
`,
		},
		{
			name:       "hw ver 15",
			hwVersions: []string{"vmx-15"},
			expectedMetrics: `
# HELP vsphere_node_hw_version_total [ALPHA] Number of vSphere nodes with given HW version.
# TYPE vsphere_node_hw_version_total gauge
vsphere_node_hw_version_total{hw_version="vmx-13"} 1
vsphere_node_hw_version_total{hw_version="vmx-15"} 1
`,
		},
		{
			name:       "if hardware version of nodes change",
			hwVersions: []string{"vmx-15", "vmx-15"},
			expectedMetrics: `
# HELP vsphere_node_hw_version_total [ALPHA] Number of vSphere nodes with given HW version.
# TYPE vsphere_node_hw_version_total gauge
vsphere_node_hw_version_total{hw_version="vmx-15"} 2
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
			ctx, cleanup, err := SetupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			collector := metrics.NewMetricsCollector()
			ctx.MetricsCollector = collector

			// Set HW version of the first VM. Leave the other VMs with the default version (vmx-13).
			if len(test.hwVersions) > 0 {
				for i := range test.hwVersions {
					node := kubeClient.nodes[i]
					err := setHardwareVersion(ctx, node, test.hwVersions[i])
					if err != nil {
						t.Fatalf("Failed to customize node: %s", err)
					}
				}
			}
			customRegistry := basemetrics.NewKubeRegistry()
			customRegistry.CustomMustRegister(collector)

			// Act - simulate loop through all nodes
			err = check.StartCheck()
			if err != nil {
				t.Errorf("StartCheck failed: %s", err)
			}

			for _, node := range kubeClient.nodes {
				vm, err := getVM(ctx, node)
				if err != nil {
					t.Errorf("Error getting vm for node %s: %s", node.Name, err)
				}
				err = check.CheckNode(ctx, node, vm)
				if err != nil {
					t.Errorf("Unexpected error on node %s: %s", node.Name, err)
				}
			}

			check.FinishCheck(ctx)
			collector.FinishedAllChecks()

			// Assert
			if err := testutil.GatherAndCompare(customRegistry, strings.NewReader(test.expectedMetrics), "vsphere_node_hw_version_total"); err != nil {
				t.Errorf("Unexpected metric: %s", err)
			}
		})
	}
}
