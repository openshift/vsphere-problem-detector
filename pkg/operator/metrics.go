package operator

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	checkNameLabel = "check"
	nodeNameLabel  = "node"
)

// This file contains operator metrics, especially status of each check.
// For other metrics exposed by this operator, see pkg/check.

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

	nodeCheckMetric = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Name:           "vsphere_node_check_duration_seconds",
			Help:           "vSphere node-level check duration",
			Buckets:        []float64{.1, .25, .5, 1, 2.5, 5, 10, 15, 30, 60, 120, 300, 600},
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel, nodeNameLabel},
	)

	nodeCheckErrrorMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "vsphere_node_check_errors",
			Help:           "Number of failed vSphere node-level checks performed by vsphere-problem-detector.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel, nodeNameLabel},
	)
)

func init() {
	legacyregistry.MustRegister(clusterCheckMetric)
	legacyregistry.MustRegister(clusterCheckErrrorMetric)
	legacyregistry.MustRegister(nodeCheckMetric)
	legacyregistry.MustRegister(nodeCheckErrrorMetric)
}
