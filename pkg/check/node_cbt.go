package check

import (
	"fmt"
	"strings"

	lmetric "github.com/openshift/vsphere-problem-detector/pkg/metrics"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/klog/v2"
)

// CollectNodeCBT emits metric with CBT Config of each VM
type CollectNodeCBT struct {
}

var _ NodeCheck = &CollectNodeCBT{}

const (
	cbtMismatchLabel = "cbt"
	CbtProperty      = "ctkEnabled"
	cbtEnabledKey    = "enabled"
	cbtDisabledKey   = "disabled"
)

func (c *CollectNodeCBT) Name() string {
	return "CollectNodeCBT"
}

func (c *CollectNodeCBT) StartCheck() error {
	return nil
}

func (c *CollectNodeCBT) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) *CheckError {
	klog.V(4).Infof("Checking CBT Property")

	propFound := false
	for _, config := range vm.Config.ExtraConfig {
		if config.GetOptionValue().Key == CbtProperty {
			klog.V(2).Infof("Found ctkEnabled property for node %v with value %v", node.Name, config.GetOptionValue().Value)
			if strings.EqualFold(fmt.Sprintf("%v", config.GetOptionValue().Value), "TRUE") {
				ctx.ClusterInfo.SetCbtData(cbtEnabledKey)
			} else {
				ctx.ClusterInfo.SetCbtData(cbtDisabledKey)
			}
			propFound = true
			break
		}
	}
	if !propFound {
		klog.V(2).Infof("Property not found for node %v", node.Name)
		ctx.ClusterInfo.SetCbtData(cbtDisabledKey)
	}

	return nil
}

func (c *CollectNodeCBT) FinishCheck(ctx *CheckContext) {
	cbtData := ctx.ClusterInfo.GetCbtData()

	klog.V(2).Infof("Enabled (%v) Disabled (%v)", cbtData[cbtEnabledKey], cbtData[cbtDisabledKey])
	// Set the counts of enabled vs disabled
	for cbtEnabled, count := range cbtData {
		klog.V(4).Infof("CBT (%v): %v", cbtEnabled, count)
		m := metrics.NewLazyConstMetric(lmetric.CbtMismatchMetric, metrics.GaugeValue, float64(count), cbtEnabled)
		ctx.MetricsCollector.AddMetric(m)
	}
}
