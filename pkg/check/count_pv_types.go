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
			Help:           "Number of vSAN volumes used by user in vSphere cluster",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{},
	)
	zonalPVCountMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_zonal_volumes_total",
			Help:           "Number of zonal vSphere volumes in cluster",
			StabilityLevel: metrics.ALPHA,
		}, []string{},
	)
	rwxVolumeRegx = regexp.MustCompile(`file\:`)
)

func init() {
	legacyregistry.MustRegister(rwxPVCountMetric)
	legacyregistry.MustRegister(zonalPVCountMetric)
}

func CountPVTypes(ctx *CheckContext) *CheckError {
	rwxPVCount, zonalPVCount, err := countVolumeTypes(ctx)
	if err != nil {
		return NewCheckError(OpenshiftAPIError, err)
	}

	rwxPVCountMetric.WithLabelValues().Set(float64(rwxPVCount))
	zonalPVCountMetric.WithLabelValues().Set(float64(zonalPVCount))
	return nil
}

func countVolumeTypes(ctx *CheckContext) (rwxCount int, zonalPVCount int, err error) {
	pvs, err := ctx.KubeClient.ListPVs(ctx.Context)
	if err != nil {
		return
	}
	for i := range pvs {
		pv := pvs[i]
		csi := pv.Spec.CSI
		if csi != nil && csi.Driver == vSphereCSIDdriver {
			volumeHandle := csi.VolumeHandle
			if rwxVolumeRegx.MatchString(volumeHandle) {
				rwxCount += 1
			}

			if pv.Spec.NodeAffinity != nil && pv.Spec.NodeAffinity.Required != nil {
				nodeSelectors := pv.Spec.NodeAffinity.Required
				if len(nodeSelectors.NodeSelectorTerms) > 0 {
					zonalPVCount += 1
				}
			}
		}
	}
	return
}
