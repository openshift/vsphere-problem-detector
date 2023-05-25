package operator

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	checkNameLabel = "check"
	nodeNameLabel  = "node"
	reasonLabel    = "reason"
)

// This file contains operator metrics, especially status of each check.
// For other metrics exposed by this operator, see pkg/check.

var (
	clusterCheckTotalMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "vsphere_cluster_check_total",
			Help:           "Number of vSphere cluster-level checks performed by vsphere-problem-detector, including both successes and failures.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel},
	)

	clusterCheckErrrorMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_cluster_check_errors",
			Help:           "Indicates failing vSphere cluster-level checks performed by vsphere-problem-detector. Value of 1 means - a particular check is failing.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel},
	)

	nodeCheckTotalMetric = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Name:           "vsphere_node_check_total",
			Help:           "Number of vSphere node-level checks performed by vsphere-problem-detector, including both successes and failures.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel, nodeNameLabel},
	)

	nodeCheckErrrorMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_node_check_errors",
			Help:           "Indicates failing vSphere node-level checks performed by vsphere-problem-detector. Value of 1 means - a particular check is failing on a node.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{checkNameLabel, nodeNameLabel},
	)

	syncErrrorMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_sync_errors",
			Help:           "Indicates failing vSphere problem detector sync error. Value 1 means that the last sync failed.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{reasonLabel},
	)

	alertsDisabledMetric = metrics.NewGauge(
		&metrics.GaugeOpts{
			Name:           "vsphere_problem_detector_disabled_alerts",
			Help:           "Indicates that vsphere-problem-detector alerts has been disabled.",
			StabilityLevel: metrics.ALPHA,
		},
	)
)

func init() {
	legacyregistry.MustRegister(clusterCheckTotalMetric)
	legacyregistry.MustRegister(clusterCheckErrrorMetric)
	legacyregistry.MustRegister(nodeCheckTotalMetric)
	legacyregistry.MustRegister(nodeCheckErrrorMetric)
	legacyregistry.MustRegister(syncErrrorMetric)
	legacyregistry.MustRegister(alertsDisabledMetric)
}
