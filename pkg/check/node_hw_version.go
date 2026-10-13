package check

import (
	lmetric "github.com/openshift/vsphere-problem-detector/pkg/metrics"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/klog/v2"
)

// CollectNodeHWVersion emits metric with HW version of each VM
type CollectNodeHWVersion struct {
}

var _ NodeCheck = &CollectNodeHWVersion{}

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

	for hwVersion, count := range hwversions {
		m := metrics.NewLazyConstMetric(lmetric.HwVersionMetric, metrics.GaugeValue, float64(count), hwVersion)
		ctx.MetricsCollector.AddMetric(m)
	}
}
