package operator

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	checkNameLabel = "check"
)

var (
	clusterCheckMetric = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Name:           "vsphere_cluster_check_duration_seconds",
			Help:           "vSphere cluster-level check duration",
			Buckets:        []float64{.1, .25, .5, 1, 2.5, 5, 10, 15, 30, 60, 120, 300, 600},
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel},
	)
	clusterCheckErrrorMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "vsphere_cluster_check_errors",
			Help:           "Number of failed vSphere cluster-level checks performed by vsphere-problem-detector.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel},
	)
)

func init() {
	legacyregistry.MustRegister(clusterCheckMetric)
	legacyregistry.MustRegister(clusterCheckErrrorMetric)
}
