package check

import (
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

const (
	versionLabel    = "version"
	buildLabel      = "build"
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
		[]string{versionLabel, apiVersionLabel, vCenterUUID, buildLabel},
	)
)

func init() {
	legacyregistry.MustRegister(vCenterInfoMetric)
}

// CollectClusterInfo grabs information about vSphere cluster and updates corresponding metrics.
// It's not a vSphere check per se, just using the interface.
func CollectClusterInfo(ctx *CheckContext) *CheckError {
	vCenterInfoMetric.Reset()
	collectVCenterInfo(ctx)
	return nil
}

func collectVCenterInfo(ctx *CheckContext) {
	version := ctx.VMClient.ServiceContent.About.Version
	apiVersion := ctx.VMClient.ServiceContent.About.ApiVersion
	build := ctx.VMClient.ServiceContent.About.Build
	uuid := ctx.VMClient.ServiceContent.About.InstanceUuid
	klog.V(2).Infof("vCenter version is %s, apiVersion is %s and build is %s", version, apiVersion, build)
	ctx.ClusterInfo.SetVCenterVersion(version, apiVersion)
	vCenterInfoMetric.WithLabelValues(version, apiVersion, uuid, build).Set(1.0)
}
