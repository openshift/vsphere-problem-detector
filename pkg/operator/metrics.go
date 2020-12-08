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
	clusterCheckErrrorMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "vsphere_cluster_check_errors",
			Help:           "Number of failed vSphere cluster-level checks performed by vsphere-problem-detector.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel},
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
	legacyregistry.MustRegister(clusterCheckErrrorMetric)
	legacyregistry.MustRegister(nodeCheckErrrorMetric)
}
