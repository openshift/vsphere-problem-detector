package check

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	versionLabel    = "version"
	apiVersionLabel = "api_version"
	vCenterUUID     = "uuid"
)

var (
	vCenterInfoMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_vcenter_info",
			Help:           "Information about vSphere vCenter.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{versionLabel, apiVersionLabel, vCenterUUID},
	)
)

func init() {
	legacyregistry.MustRegister(vCenterInfoMetric)
}

// CollectClusterInfo grabs information about vSphere cluster and updates corresponding metrics.
// It's not a vSphere check per se, just using the interface.
func CollectClusterInfo(ctx *CheckContext) error {
	vCenterInfoMetric.Reset()
	collectVCenterInfo(ctx)
	return nil
}

func collectVCenterInfo(ctx *CheckContext) {
	version := ctx.VMClient.ServiceContent.About.Version
	apiVersion := ctx.VMClient.ServiceContent.About.ApiVersion
	uuid := ctx.VMClient.ServiceContent.About.InstanceUuid
	vCenterInfoMetric.WithLabelValues(version, apiVersion, uuid).Set(1.0)
}
