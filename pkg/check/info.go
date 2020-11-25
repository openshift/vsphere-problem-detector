package check

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	versionLabel = "version"
	vCenterUUID  = "uuid"
)

var (
	vCenterInfoMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_vcenter_info",
			Help:           "Information about vSphere vCenter.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{versionLabel, vCenterUUID},
	)
)

func init() {
	legacyregistry.MustRegister(vCenterInfoMetric)
}

// CollectClusterInfo grabs information about vSphere cluster and updates corresponding metrics.
// It's not a vSphere check per se, just using the interface.
func CollectClusterInfo(ctx *CheckContext) error {
	collectVCenterInfo(ctx)
	return nil
}

func collectVCenterInfo(ctx *CheckContext) {
	version := ctx.VMClient.ServiceContent.About.Version
	uuid := ctx.VMClient.ServiceContent.About.InstanceUuid
	vCenterInfoMetric.WithLabelValues(version, uuid).Set(1.0)
}
