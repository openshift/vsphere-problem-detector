package check

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi/vim25/mo"
	vim25types "github.com/vmware/govmomi/vim25/types"

	"k8s.io/klog/v2"
)

// CheckHostGroups will check the infrastructure spec for zones which use host groups.
// if any are found, the nodes in those host groups will be checked to ensure they are
// tagged appropriately
func CheckHostGroups(ctx *CheckContext) error {
	var errs []error

	klog.V(4).Info("checking host groups for required tagging")

	// Get all failure domains
	klog.V(4).Info("getting infrastructure configuration")
	inf, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		return fmt.Errorf("error getting infrastructure: %v", err)
	}

	// Perform check if FailureDomains defined.  We need 2 or more to require tags.
	klog.V(4).Info("checking failure domains")

	var fds []configv1.VSpherePlatformFailureDomainSpec

	// In existing UPI clusters the VSphere field in the infra spec
	// may not be defined.
	if inf.Spec.PlatformSpec.VSphere != nil {
		fds = inf.Spec.PlatformSpec.VSphere.FailureDomains
	} else {
		klog.V(2).Infof("VSphere Infrastructure spec is empty, no FailureDomains configured. Skipping check.")
		return nil
	}

	for _, fd := range fds {
		if fd.ZoneAffinity == nil ||
			(fd.ZoneAffinity.Type != configv1.HostGroupFailureDomainZone) {
			continue
		}

		var vcenter *VCenter
		var found bool
		if vcenter, found = ctx.VCenters[fd.Server]; !found {
			return fmt.Errorf("vCenter %s not defined", fd.Server)
		}

		_, zoneTagCategoryID, err := validateTagCategories(ctx, vcenter)
		if err != nil {
			return fmt.Errorf("unable to get zone tag category from vCenter %s", fd.Server)
		}

		dc, err := getDatacenter(ctx, vcenter, fd.Topology.Datacenter)
		if err != nil {
			return fmt.Errorf("unable to get datacenter: %s in %s: %v",
				fd.Topology.Datacenter,
				fd.Server,
				err)
		}
		ccr, err := getClusterComputeResource(ctx, vcenter, fd.Topology.ComputeCluster, dc)
		if err != nil {
			return fmt.Errorf("unable to get compute cluster: %s in %s: %v",
				fd.Topology.ComputeCluster,
				fd.Server,
				err)
		}

		clusterConfig, err := ccr.Configuration(ctx.Context)
		if err != nil {
			return fmt.Errorf("unable to get compute cluster configuration: %s in %s: %v",
				fd.Topology.ComputeCluster,
				fd.Server,
				err)
		}

		var hostGroupPresent, vmGroupPresent, vmRulePresent bool
		var hostsInGroup []vim25types.ManagedObjectReference

		for _, group := range clusterConfig.Group {
			groupInfo := group.GetClusterGroupInfo()

			switch groupType := group.(type) {
			case *vim25types.ClusterHostGroup:
				if groupInfo.Name != fd.ZoneAffinity.HostGroup.HostGroup {
					continue
				}
				hostGroupPresent = true
				hostsInGroup = append(hostsInGroup, group.(*vim25types.ClusterHostGroup).Host...)

			case *vim25types.ClusterVmGroup:
				if groupInfo.Name == fd.ZoneAffinity.HostGroup.VMGroup {
					vmGroupPresent = true
				}

			default:
				klog.V(2).Infof("unsupported group type %v", groupType)
			}
		}

		for _, rule := range clusterConfig.Rule {
			ruleInfo := rule.GetClusterRuleInfo()
			switch ruleType := rule.(type) {
			case *vim25types.ClusterVmHostRuleInfo:
				if ruleInfo.Name == fd.ZoneAffinity.HostGroup.VMHostRule {
					vmRulePresent = true
				}
				break
			default:
				klog.V(2).Infof("unsupported rule type %v", ruleType)
			}
		}

		if !hostGroupPresent {
			errs = append(errs, fmt.Errorf("%s refers to host group %s which is not found", fd.Name, fd.ZoneAffinity.HostGroup.HostGroup))
		} else if len(hostsInGroup) == 0 {
			errs = append(errs, fmt.Errorf("%s does not have any hosts in host group %s", fd.Name, fd.ZoneAffinity.HostGroup.HostGroup))
		} else {
			var references []mo.Reference
			unmatchedReferenceMap := make(map[string]mo.Reference)

			for _, host := range hostsInGroup {
				references = append(references, host.Reference())
				unmatchedReferenceMap[host.Value] = host
			}

			attachedTags, err := vcenter.TagManager.GetAttachedTagsOnObjects(ctx.Context, references)
			if err != nil {
				errs = append(errs, fmt.Errorf("unable to get tags for hosts in host group %s %v", fd.ZoneAffinity.HostGroup.HostGroup, err))
			}
			for _, attachedTag := range attachedTags {
				if _, present := unmatchedReferenceMap[attachedTag.ObjectID.Reference().Value]; !present {
					continue
				}

				for _, tag := range attachedTag.Tags {
					if tag.CategoryID == zoneTagCategoryID && tag.Name == fd.Zone {
						delete(unmatchedReferenceMap, attachedTag.ObjectID.Reference().Value)
					}
				}
			}

			if len(unmatchedReferenceMap) > 0 {
				for _, host := range unmatchedReferenceMap {
					errs = append(errs, fmt.Errorf("host %s in host group %s does not have an openshift-zone tag with value %s", host.Reference().Value, fd.ZoneAffinity.HostGroup.HostGroup, fd.Zone))
				}
			}

		}

		if !vmGroupPresent {
			errs = append(errs, fmt.Errorf("%s refers to vm group %s which is not found", fd.Name, fd.ZoneAffinity.HostGroup.VMGroup))
		}

		if !vmRulePresent {
			errs = append(errs, fmt.Errorf("%s refers to vm host rule %s which is not found", fd.Name, fd.ZoneAffinity.HostGroup.VMHostRule))
		}
	}
	if errs != nil {
		return joinWithSeparator(errs, ";")
	}
	return nil
}
