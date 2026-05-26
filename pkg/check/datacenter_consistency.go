package check

import (
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

// CheckDatacenterConsistency validates that every datacenter referenced by a failure domain
// in the Infrastructure CR is also listed in the cloud provider config (cloud.conf).
// A mismatch means the CSI driver cannot find VMs in the affected zone.
func CheckDatacenterConsistency(ctx *CheckContext) error {
	klog.Info("Checking datacenter consistency between failure domains and cloud provider config.")

	infra, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		klog.Errorf("Error getting infrastructure: %v", err)
		return err
	}

	if infra.Spec.PlatformSpec.VSphere == nil || len(infra.Spec.PlatformSpec.VSphere.FailureDomains) == 0 {
		klog.V(2).Info("Skipping datacenter consistency check due to legacy infra config.")
		return nil
	}

	var errs []error
	cfg := ctx.VMConfig.Config

	for _, fd := range infra.Spec.PlatformSpec.VSphere.FailureDomains {
		vcConfig := cfg.VirtualCenter[fd.Server]
		if vcConfig == nil {
			klog.V(4).Infof("vCenter %q not found in cloud provider config for failure domain %q, skipping (handled by CheckInfraConfig)", fd.Server, fd.Name)
			continue
		}

		configuredDCs := parseDatacenters(vcConfig.Datacenters)
		if !containsString(configuredDCs, fd.Topology.Datacenter) {
			err := fmt.Errorf("Datacenter-Consistency: failure domain %q (infrastructure.config.openshift.io/cluster) "+
				"requires datacenter %q on vCenter %q, but it is not listed in the cloud provider config "+
				"(datacenters = %q in vsphere-csi-config-secret, namespace openshift-cluster-csi-drivers). "+
				"Add %q to the datacenters list in the cloud-provider-config ConfigMap in the %s namespace.",
				fd.Name, fd.Topology.Datacenter, fd.Server,
				vcConfig.Datacenters,
				fd.Topology.Datacenter, util.CloudConfigNamespace)
			errs = append(errs, err)
			klog.Warningf("%v", err)
		}
	}

	for _, fd := range infra.Spec.PlatformSpec.VSphere.FailureDomains {
		for _, vc := range infra.Spec.PlatformSpec.VSphere.VCenters {
			if vc.Server == fd.Server {
				if !containsString(vc.Datacenters, fd.Topology.Datacenter) {
					err := fmt.Errorf("Datacenter-Consistency: failure domain %q references datacenter %q "+
						"on vCenter %q, but it is not listed in the vcenters section of "+
						"infrastructure.config.openshift.io/cluster (datacenters = %v). "+
						"Add %q to the datacenters list for vCenter %q in the Infrastructure CR.",
						fd.Name, fd.Topology.Datacenter, fd.Server,
						vc.Datacenters, fd.Topology.Datacenter, fd.Server)
					errs = append(errs, err)
					klog.Warningf("%v", err)
				}
				break
			}
		}
	}

	if len(errs) > 0 {
		return join(errs)
	}
	return nil
}

func parseDatacenters(raw string) []string {
	var result []string
	for _, dc := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(dc)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
