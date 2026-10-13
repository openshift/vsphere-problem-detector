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
func CollectClusterInfo(ctx *CheckContext) error {
	vCenterInfoMetric.Reset()
	collectVCenterInfo(ctx)
	return nil
}

func collectVCenterInfo(ctx *CheckContext) {
	for _, vCenter := range ctx.VCenters {
		version := vCenter.VMClient.ServiceContent.About.Version
		apiVersion := vCenter.VMClient.ServiceContent.About.ApiVersion
		build := vCenter.VMClient.ServiceContent.About.Build
		uuid := vCenter.VMClient.ServiceContent.About.InstanceUuid
		klog.V(2).Infof("vCenter %s version is %s, apiVersion is %s and build is %s", vCenter.VCenterName, version, apiVersion, build)
		ctx.ClusterInfo.SetVCenterVersion(vCenter.VCenterName, version, apiVersion)
		vCenterInfoMetric.WithLabelValues(version, apiVersion, uuid, build).Set(1.0)
	}
}
