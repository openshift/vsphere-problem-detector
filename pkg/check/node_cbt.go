package check

import (
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

// CollectNodeCBT emits metric with CBT Config of each VM
type CollectNodeCBT struct {
	lastMetricEmission map[string]int
}

var _ NodeCheck = &CollectNodeCBT{}

const (
	cbtMismatchLabel = "cbt"
	CbtProperty      = "ctkEnabled"
	cbtEnabled       = "enabled"
	cbtDisabled      = "disabled"
)

var (
	cbtMismatchMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_vm_cbt_checks",
			Help:           "Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{cbtMismatchLabel},
	)
)

func init() {
	legacyregistry.MustRegister(cbtMismatchMetric)
}

func (c *CollectNodeCBT) Name() string {
	return "CollectNodeCBT"
}

func (c *CollectNodeCBT) StartCheck() error {
	return nil
}

func (c *CollectNodeCBT) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	klog.V(4).Infof("Checking CBT Property")

	propFound := false
	for _, config := range vm.Config.ExtraConfig {
		if config.GetOptionValue().Key == CbtProperty {
			klog.V(2).Infof("Found ctkEnabled property for node %v with value %v", node.Name, config.GetOptionValue().Value)
			if strings.EqualFold(fmt.Sprintf("%v", config.GetOptionValue().Value), "TRUE") {
				ctx.ClusterInfo.SetCbtData(cbtEnabled)
			} else {
				ctx.ClusterInfo.SetCbtData(cbtDisabled)
			}
			propFound = true
			break
		}
	}
	if !propFound {
		klog.V(2).Infof("Property not found for node %v", node.Name)
		ctx.ClusterInfo.SetCbtData(cbtDisabled)
	}

	return nil
}

func (c *CollectNodeCBT) FinishCheck(ctx *CheckContext) {
	cbtData := ctx.ClusterInfo.GetCbtData()

	for k := range c.lastMetricEmission {
		c.lastMetricEmission[k] = 0
	}

	klog.V(2).Infof("Enabled (%v) Disabled (%v)", cbtData[cbtEnabled], cbtData[cbtDisabled])
	// Set the counts of enabled vs disabled
	for cbtEnabled, count := range cbtData {
		klog.V(4).Infof("CBT (%v): %v", cbtEnabled, count)
		cbtMismatchMetric.WithLabelValues(cbtEnabled).Set(float64(count))
	}

	for k, v := range c.lastMetricEmission {
		if v == 0 {
			cbtMismatchMetric.WithLabelValues(k).Set(0)
		}
	}
}
