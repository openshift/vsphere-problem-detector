package check

import (
	"sync"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

// CollectNodeHWVersion emits metric with HW version of each VM
type CollectNodeHWVersion struct {
	// map HW version -> count of nodes with this version
	hwVersions     map[string]int
	hwVersionsLock sync.Mutex
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
	c.hwVersionsLock.Lock()
	defer c.hwVersionsLock.Unlock()
	c.hwVersions = make(map[string]int)
	return nil
}

func (c *CollectNodeHWVersion) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	hwVersion := vm.Config.Version
	klog.V(2).Infof("Node %s has HW version %s", node.Name, hwVersion)

	c.hwVersionsLock.Lock()
	defer c.hwVersionsLock.Unlock()

	c.hwVersions[hwVersion]++
	return nil
}

func (c *CollectNodeHWVersion) FinishCheck() {
	c.hwVersionsLock.Lock()
	defer c.hwVersionsLock.Unlock()

	for hwVersion, count := range c.hwVersions {
		hwVersionMetric.WithLabelValues(hwVersion).Set(float64(count))
	}
	return
}
