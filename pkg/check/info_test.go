package check

import (
	"strings"
	"testing"

	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/component-base/metrics/testutil"
)

func TestInfo(t *testing.T) {
	ctx, cleanup, err := setupSimulator(nil, defaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	err = CollectClusterInfo(ctx)
	if err != nil {
		t.Errorf("CollectClusterInfo failed: %s", err)
	}

	// UUID & version is hardcoded in the simulator, github.com/vmware/govmomi/simulator/vpx/service_content.go
	expectedMetric := `
        # HELP vsphere_vcenter_info [ALPHA] Information about vSphere vCenter.
        # TYPE vsphere_vcenter_info gauge
        vsphere_vcenter_info{uuid="dbed6e0c-bd88-4ef6-b594-21283e1c677f",version="6.5.0"} 1
`

	if err := testutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(expectedMetric), "vsphere_vcenter_info"); err != nil {
		t.Errorf("Unexpected metric: %s", err)
	}
}
