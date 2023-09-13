package metrics

import (
	"sync"

	"k8s.io/component-base/metrics"
)

type Collector struct {
	metrics.BaseStableCollector
	lock sync.RWMutex

	currentMetrics    []metrics.Metric
	inProgressMetrics []metrics.Metric
	markStaleMetrics  bool
}

var _ metrics.StableCollector = &Collector{}

const (
	VersionLabel    = "version"
	BuildLabel      = "build"
	ApiVersionLabel = "api_version"
	vCenterUUID     = "uuid"

	HwVersionLabel   = "hw_version"
	cbtMismatchLabel = "cbt"
)

// Metrics in this package can be used when we expect metrics with certain label to stop
// being reported when cluster configuration changes.
var (
	EsxiVersionMetric = metrics.NewDesc(
		"vsphere_esxi_version_total",
		"Number of ESXi hosts with given version.",
		[]string{VersionLabel, ApiVersionLabel}, nil,
		metrics.ALPHA, "",
	)
	HwVersionMetric = metrics.NewDesc(
		"vsphere_node_hw_version_total",
		"Number of vSphere nodes with given HW version.",
		[]string{HwVersionLabel}, nil,
		metrics.ALPHA, "",
	)

	CbtMismatchMetric = metrics.NewDesc(
		"vsphere_vm_cbt_checks",
		"Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.",
		[]string{cbtMismatchLabel}, nil,
		metrics.ALPHA, "",
	)
)

func NewMetricsCollector() *Collector {
	return &Collector{
		currentMetrics:    []metrics.Metric{},
		inProgressMetrics: []metrics.Metric{},
	}
}

func (c *Collector) DescribeWithStability(ch chan<- *metrics.Desc) {
	ch <- EsxiVersionMetric
	ch <- HwVersionMetric
	ch <- CbtMismatchMetric
}

func (c *Collector) CollectWithStability(ch chan<- metrics.Metric) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, m := range c.currentMetrics {
		ch <- m
	}
}

func (c *Collector) AddMetric(m metrics.Metric) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.inProgressMetrics = append(c.inProgressMetrics, m)
}

func (c *Collector) StartMetricCollection() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.inProgressMetrics = []metrics.Metric{}
}

// FinishedAllChecks updates currentMetrics with inProgressMetrics so as
// both slices point to same values
func (c *Collector) FinishedAllChecks() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.currentMetrics = c.inProgressMetrics
}
