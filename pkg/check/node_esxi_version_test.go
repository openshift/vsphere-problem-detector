package check

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/vmware/govmomi/vim25/mo"
	basemetrics "k8s.io/component-base/metrics"

	"github.com/openshift/vsphere-problem-detector/pkg/metrics"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

func TestCollectNodeESXiVersion(t *testing.T) {
	tests := []struct {
		name            string
		esxiVersion     string
		esxiApiversion  string
		expectedMetrics string
	}{
		{
			name:           "esxi 6.7.0",
			esxiVersion:    "6.7.0",
			esxiApiversion: "6.7.3",
			expectedMetrics: `
        # HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
        # TYPE vsphere_esxi_version_total gauge
        vsphere_esxi_version_total{api_version="6.7.3", version="6.7.0"} 1
`,
		},
		{
			name:           "esxi 7.0.0",
			esxiVersion:    "7.0.0",
			esxiApiversion: "7.0.0-1",
			expectedMetrics: `
        # HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
        # TYPE vsphere_esxi_version_total gauge
        vsphere_esxi_version_total{api_version="7.0.0-1", version="7.0.0"} 1
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			check := CollectNodeESXiVersion{}
			kubeClient := &testlib.FakeKubeClient{
				Nodes: testlib.DefaultNodes(),
			}
			ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()
			collector := metrics.NewMetricsCollector()
			ctx.MetricsCollector = collector

			// Set esxi version of the only host.
			if len(ctx.VCenters) == 0 {
				t.Fatalf("No vCenters found")
			}
			for _, vCenter := range ctx.VCenters {
				err = testlib.CustomizeHostVersion(vCenter.Model.Service.Context, testlib.DefaultHostId, test.esxiVersion, test.esxiApiversion)
				if err != nil {
					t.Fatalf("Failed to customize host: %s", err)
				}
				break // Only process the first vCenter
			}

			// Reset metrics from previous tests. Note: the tests can't run in parallel!
			// legacyregistry.Reset()
			customRegistry := basemetrics.NewKubeRegistry()
			customRegistry.CustomMustRegister(collector)

			// Act - simulate loop through all nodes
			err = check.StartCheck()
			if err != nil {
				t.Errorf("StartCheck failed: %s", err)
			}

			for _, node := range kubeClient.Nodes {
				var vCenter *VCenter
				var vm *mo.VirtualMachine

				// Get vCenter
				vCenter, err = GetVCenter(ctx, node)
				if err != nil {
					t.Errorf("Error getting vCenter for node %s: %s", node.Name, err)
				}

				if err != nil {
					t.Errorf("Error getting vCenter for node %s: %s", node.Name, err)
				}

				// Get VM
				vm, err := testlib.GetVM(vCenter.VMClient, node)
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
			if err := testutil.GatherAndCompare(customRegistry, strings.NewReader(test.expectedMetrics), "vsphere_esxi_version_total"); err != nil {
				t.Errorf("Unexpected metric: %s", err)
			}
		})
	}
}
