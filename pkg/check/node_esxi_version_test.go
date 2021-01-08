package check

import (
	"strings"
	"testing"

	testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"k8s.io/component-base/metrics/legacyregistry"
)

func TestCollectNodeESXiVersion(t *testing.T) {
	tests := []struct {
		name            string
		esxiVersion     string
		expectedMetrics string
	}{
		{
			name:        "esxi 6.7.0",
			esxiVersion: "6.7.0",
			expectedMetrics: `
        # HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
        # TYPE vsphere_esxi_version_total gauge
        vsphere_esxi_version_total{version="6.7.0"} 1
`,
		},
		{
			name:        "esxi 7.0.0",
			esxiVersion: "7.0.0",
			expectedMetrics: `
        # HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
        # TYPE vsphere_esxi_version_total gauge
        vsphere_esxi_version_total{version="7.0.0"} 1
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			check := CollectNodeESXiVersion{}
			kubeClient := &fakeKubeClient{
				nodes: defaultNodes(),
			}
			ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Set esxi version of the only host.
			err = customizeHostVersion(defaultHostId, test.esxiVersion)
			if err != nil {
				t.Fatalf("Failed to customize host: %s", err)
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
			if err := testutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(test.expectedMetrics), "vsphere_esxi_version_total"); err != nil {
				t.Errorf("Unexpected metric: %s", err)
			}
		})
	}
}
