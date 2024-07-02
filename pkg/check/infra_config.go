package check

import (
	"fmt"

	v1 "github.com/openshift/api/config/v1"
	"k8s.io/cloud-provider-vsphere/pkg/common/config"
	"k8s.io/klog/v2"
)

// CheckInfraConfig will attempt to validate that the infrastructure config is correctly configured.
func CheckInfraConfig(ctx *CheckContext) error {
	klog.Info("Checking infrastructure and cloud provider config for consistency.")
	var errs []error

	// Get Infrastructure
	klog.V(4).Info("Getting infrastructure configuration.")
	infra, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		klog.Errorf("Error getting infrastructure: %v", err)
		return err
	}

	// If infra has 0 FDs, this is legacy config.  We'll just return and not check anything
	if infra.Spec.PlatformSpec.VSphere == nil || len(infra.Spec.PlatformSpec.VSphere.FailureDomains) == 0 {
		klog.V(2).Infof("Skipping infra check due to legacy infra config.")
		return nil
	}

	// Check that the vCenters are configured in both infra and config
	errs = append(errs, checkVCenters(ctx, infra)...)

	// Check failure domains
	errs = append(errs, checkFailureDomains(ctx, infra)...)

	if len(errs) > 0 {
		return join(errs)
	}
	return nil
}

func checkVCenters(ctx *CheckContext, infra *v1.Infrastructure) []error {
	var errs []error
	// Check that the vCenters are configured in both infra and config
	vSphereSpec := infra.Spec.PlatformSpec.VSphere
	cfg := ctx.VMConfig.Config
	if len(vSphereSpec.VCenters) != len(cfg.VirtualCenter) {
		err := fmt.Errorf("Infra-Config: vCenter counts do not match.  Infra has %d %v and cloud provider config has %d %v",
			len(vSphereSpec.VCenters), getInfraVCenterNames(vSphereSpec), len(cfg.VirtualCenter), getProviderConfigVCenterNames(cfg))
		errs = append(errs, err)

	} else {
		// Count is the same.  Let's verify they all match.
		for _, vCenter := range vSphereSpec.VCenters {
			foundVC := cfg.VirtualCenter[vCenter.Server]
			if foundVC == nil {
				err := fmt.Errorf("Infra-Config: vCenter values do not match.  Infra has %v and cloud provider config has %v",
					getInfraVCenterNames(vSphereSpec), getProviderConfigVCenterNames(cfg))
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func checkFailureDomains(ctx *CheckContext, infra *v1.Infrastructure) []error {
	var errs []error

	// Check each failure domain and see if the vCenter (server) reference is valid.
	foundVCs := make(map[string]bool)
	vSphereSpec := infra.Spec.PlatformSpec.VSphere
	for _, fd := range vSphereSpec.FailureDomains {
		if !containsString(getInfraVCenterNames(vSphereSpec), fd.Server) {
			err := fmt.Errorf("Infra-Config: failure domain %v references the server %v but is not found in the vCenter section %v",
				fd.Name, fd.Server, getInfraVCenterNames(vSphereSpec))
			errs = append(errs, err)
		} else {
			// Ok, FD has valid server defined.
			foundVCs[fd.Server] = true
		}
	}

	// Verify all vCenters were referenced by failure domains
	for _, vCenter := range vSphereSpec.VCenters {
		if foundVCs[vCenter.Server] != true {
			err := fmt.Errorf("Infra-Config: vCenter %v is configured in the infrastructure resource but not used by any failure domains",
				vCenter.Server)
			errs = append(errs, err)
		}
	}

	return errs
}

func getInfraVCenterNames(infra *v1.VSpherePlatformSpec) []string {
	var vCenterNames []string
	for _, vCenter := range infra.VCenters {
		vCenterNames = append(vCenterNames, vCenter.Server)
	}
	return vCenterNames
}

func getProviderConfigVCenterNames(cfg *config.Config) []string {
	var vCenterNames []string
	for vCenterName := range cfg.VirtualCenter {
		vCenterNames = append(vCenterNames, vCenterName)
	}
	return vCenterNames
}

func containsString(array []string, searchStr string) bool {
	for _, val := range array {
		if val == searchStr {
			return true
		}
	}
	return false
}
