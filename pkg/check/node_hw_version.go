package check

import (
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

// CollectNodeHWVersion emits metric with HW version of each VM
type CollectNodeHWVersion struct {
	lastMetricEmission map[string]int
}

var _ NodeCheck = &CollectNodeHWVersion{}

const (
	hwVersionLabel = "hw_version"
)

var (
	hwVersionMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_node_hw_version_total",
			Help:           "Number of vSphere nodes with given HW version.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{hwVersionLabel},
	)
)

func init() {
	legacyregistry.MustRegister(hwVersionMetric)
}

func (c *CollectNodeHWVersion) Name() string {
	return "CollectNodeHWVersion"
}

func (c *CollectNodeHWVersion) StartCheck() error {
	return nil
}

func (c *CollectNodeHWVersion) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	hwVersion := vm.Config.Version
	klog.V(2).Infof("Node %s has HW version %s", node.Name, hwVersion)
	ctx.ClusterInfo.SetHardwareVersion(hwVersion)
	return nil
}

func (c *CollectNodeHWVersion) FinishCheck(ctx *CheckContext) {
	hwversions := ctx.ClusterInfo.GetHardwareVersion()

	for k := range c.lastMetricEmission {
		c.lastMetricEmission[k] = 0
	}

	for hwVersion, count := range hwversions {
		c.lastMetricEmission[hwVersion] = count
		hwVersionMetric.WithLabelValues(hwVersion).Set(float64(count))
	}

	for k, v := range c.lastMetricEmission {
		if v == 0 {
			hwVersionMetric.WithLabelValues(k).Set(0)
		}
	}
	return
}
