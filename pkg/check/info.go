package check

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	"k8s.io/klog/v2"
)

const (
	featureLabel = "feature"
	versionLabel = "version"
	vCenterUUID  = "uuid"

	featureDRS = "drs_enabled"
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
	featuresMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_features",
			Help:           "Features enabled in the cluster.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{featureLabel},
	)
)

func init() {
	legacyregistry.MustRegister(vCenterInfoMetric)
	legacyregistry.MustRegister(featuresMetric)
}

// CollectClusterInfo grabs information about vSphere cluster and updates corresponding metrics.
// It's not a vSphere check per se, just using the interface.
func CollectClusterInfo(ctx *CheckContext) error {
	collectVCenterInfo(ctx)
	return collectFeatures(ctx)
}

func collectVCenterInfo(ctx *CheckContext) {
	version := ctx.VMClient.ServiceContent.About.Version
	uuid := ctx.VMClient.ServiceContent.About.InstanceUuid
	vCenterInfoMetric.WithLabelValues(version, uuid).Set(1.0)
}

func collectFeatures(ctx *CheckContext) error {
	finder := find.NewFinder(ctx.VMClient, false)

	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	dc, err := finder.Datacenter(tctx, ctx.VMConfig.Workspace.Datacenter)
	if err != nil {
		return fmt.Errorf("failed to access datacenter %s: %s", ctx.VMConfig.Workspace.Datacenter, err)
	}

	finder.SetDatacenter(dc)

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	ccr, err := finder.DefaultClusterComputeResource(tctx)
	if err != nil {
		return fmt.Errorf("failed to find the default Cluster: %s", err)
	}

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	cfg, err := ccr.Configuration(tctx)
	if err != nil {
		return err
	}

	if cfg.DrsConfig.Enabled != nil {
		if *cfg.DrsConfig.Enabled {
			klog.V(2).Infof("ClusterInfo: DRS is enabled")
			featuresMetric.WithLabelValues(featureDRS).Set(1.0)
		} else {
			klog.V(2).Infof("ClusterInfo: DRS is disabled")
			featuresMetric.WithLabelValues(featureDRS).Set(0.0)
		}
	} else {
		klog.V(2).Infof("ClusterInfo: DRS is unknown, assuming disabled")
		featuresMetric.WithLabelValues(featureDRS).Set(0.0)
	}
	return nil
}
