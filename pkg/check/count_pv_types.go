package check

import (
	"regexp"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	vSphereCSIDdriver = "csi.vsphere.vmware.com"
)

var (
	rwxPVCountMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_rwx_volumes_total",
			Help:           "Number of RWX volumes used by vSphere.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{},
	)
	rwxVolumeRegx = regexp.MustCompile(`file\:`)
)

func init() {
	legacyregistry.MustRegister(rwxPVCountMetric)
}

func CountRWXVolumes(ctx *CheckContext) error {
	rwxPVCount, err := countRWXPVsFromCluster(ctx)
	if err != nil {
		return err
	}
	rwxPVCountMetric.WithLabelValues().Set(float64(rwxPVCount))
	return nil
}

func countRWXPVsFromCluster(ctx *CheckContext) (int, error) {
	rwxPVCount := 0
	pvs, err := ctx.KubeClient.ListPVs(ctx.Context)
	if err != nil {
		return rwxPVCount, err
	}

	for i := range pvs {
		pv := pvs[i]
		csi := pv.Spec.CSI
		if csi != nil && csi.Driver == vSphereCSIDdriver {
			volumeHandle := csi.VolumeHandle
			if rwxVolumeRegx.MatchString(volumeHandle) {
				rwxPVCount += 1
			}
		}
	}
	return rwxPVCount, nil
}
