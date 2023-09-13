package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"k8s.io/component-base/metrics"
)

func TestCollector(t *testing.T) {
	tests := []struct {
		name             string
		collectorModFunc func(c *Collector) *Collector
		emittedMetric    func() metrics.Metric
		expectedMetrics  string
	}{
		{
			name: "should report old metrics when checks are still running",
			collectorModFunc: func(c *Collector) *Collector {
				c.StartMetricCollection()
				return c
			},
			emittedMetric: func() metrics.Metric {
				return metrics.NewLazyConstMetric(EsxiVersionMetric, metrics.GaugeValue, float64(10), "7.0", "7.0.2")
			},
			expectedMetrics: `
			# HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
			# TYPE vsphere_esxi_version_total gauge
			vsphere_esxi_version_total{api_version="7.0.2", version="7.0"} 10
			`,
		},
		{
			name: "marking metrics stale twice should still result in metrics being reported",
			collectorModFunc: func(c *Collector) *Collector {
				c.StartMetricCollection()
				c.StartMetricCollection()
				return c
			},
			emittedMetric: func() metrics.Metric {
				return metrics.NewLazyConstMetric(EsxiVersionMetric, metrics.GaugeValue, float64(10), "7.0", "7.0.2")
			},
			expectedMetrics: `
			# HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
			# TYPE vsphere_esxi_version_total gauge
			vsphere_esxi_version_total{api_version="7.0.2", version="7.0"} 10
			`,
		},
		{
			name: "finishing all checks should result in fresh metrics",
			collectorModFunc: func(c *Collector) *Collector {
				c.StartMetricCollection()
				m := metrics.NewLazyConstMetric(EsxiVersionMetric, metrics.GaugeValue, float64(10), "8.0", "8.0.2")
				c.AddMetric(m)
				c.FinishedAllChecks()
				return c
			},
			emittedMetric: func() metrics.Metric {
				return metrics.NewLazyConstMetric(EsxiVersionMetric, metrics.GaugeValue, float64(10), "7.0", "7.0.2")
			},
			expectedMetrics: `
			# HELP vsphere_esxi_version_total [ALPHA] Number of ESXi hosts with given version.
			# TYPE vsphere_esxi_version_total gauge
			vsphere_esxi_version_total{api_version="8.0.2", version="8.0"} 10
			`,
		},
	}

	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			collector := NewMetricsCollector()
			ch := make(chan metrics.Metric)

			customRegistry := metrics.NewKubeRegistry()
			customRegistry.CustomMustRegister(collector)

			collectedMetrics := []metrics.Metric{}

			go func() {
				for m := range ch {
					collectedMetrics = append(collectedMetrics, m)
				}
			}()
			collector.AddMetric(test.emittedMetric())
			collector.FinishedAllChecks()

			collector = test.collectorModFunc(collector)

			collector.CollectWithStability(ch)
			close(ch)
			// Assert
			if err := testutil.GatherAndCompare(customRegistry, strings.NewReader(test.expectedMetrics), "vsphere_esxi_version_total"); err != nil {
				t.Errorf("Unexpected metric: %s", err)
			}
		})
	}
}
