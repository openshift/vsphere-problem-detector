package check

import (
	"fmt"
	"regexp"

	"github.com/vmware/govmomi/vim25/mo"
	vim25types "github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"

	v1 "github.com/openshift/api/config/v1"
)

type validationContext struct {
	reference           vim25types.ManagedObjectReference
	regionTagCategoryID string
	zoneTagCategoryID   string
}

const (
	// TagCategoryRegion the tag category associated with regions.
	TagCategoryRegion = "openshift-region"

	// TagCategoryZone the tag category associated with zones.
	TagCategoryZone = "openshift-zone"
)

// validateTagCategories will verify that the categories exist for the region
// and zone tags.
func validateTagCategories(ctx *CheckContext) (string, string, *CheckError) {
	klog.V(4).Info("Validating tag categories...")
	categories, err := getCategories(ctx)
	if err != nil {
		return "", "", NewCheckError(FailedGettingTagsCategories, err)
	}

	regionTagCategoryID := ""
	zoneTagCategoryID := ""
	for _, category := range categories {
		switch category.Name {
		case TagCategoryRegion:
			regionTagCategoryID = category.ID
		case TagCategoryZone:
			zoneTagCategoryID = category.ID
		}
		if len(zoneTagCategoryID) > 0 && len(regionTagCategoryID) > 0 {
			break
		}
	}

	klog.V(4).Infof("Determined RegionTagCategoryID='%s' and ZoneTagCategoryID='%s'",
		regionTagCategoryID, zoneTagCategoryID)

	if len(zoneTagCategoryID) == 0 || len(regionTagCategoryID) == 0 {
		return "", "", NewCheckError(MissingTagsCategories, fmt.Errorf("tag categories openshift-zone and openshift-region must be created"))
	}
	return regionTagCategoryID, zoneTagCategoryID, nil
}

// validateTagAttachment will attempt to validate if region and zone tag is present
// in the ancestry of the ManagedObjectRefence that is provided in the
// validationContext.
func validateTagAttachment(ctx *CheckContext, vctx validationContext) *CheckError {
	klog.V(2).Infof("Validating tags for %s.", vctx.reference)
	referencesToCheck := []mo.Reference{vctx.reference}
	ancestors, err := getAncestors(ctx, vctx.reference)
	if err != nil {
		klog.Error("Unable to get ancestors.")
		return NewCheckError(FailedGettingTagsCategories, err)
	}

	for _, ancestor := range ancestors {
		referencesToCheck = append(referencesToCheck, ancestor.Reference())
	}
	attachedTags, err := getAttachedTagsOnObjects(ctx, referencesToCheck)
	if err != nil {
		klog.Error("Unable to get attached tags.")
		return NewCheckError(FailedGettingTagsCategories, err)
	}

	klog.V(4).Infof("Processing attached tags")
	regionTagAttached := false
	zoneTagAttached := false
	for _, attachedTag := range attachedTags {
		for _, tag := range attachedTag.Tags {
			klog.V(5).Infof("Current tag: %s", tag)
			if !regionTagAttached {
				if tag.CategoryID == vctx.regionTagCategoryID {
					regionTagAttached = true
					klog.V(4).Infof("Found Region: %s", tag.Name)
				}
			}
			if !zoneTagAttached {
				if tag.CategoryID == vctx.zoneTagCategoryID {
					zoneTagAttached = true
					klog.V(4).Infof("Found Zone: %s", tag.Name)
				}
			}
			if regionTagAttached && zoneTagAttached {
				return nil
			}
		}
	}

	klog.V(4).Infof("Region %t Zone %t", regionTagAttached, zoneTagAttached)
	if !regionTagAttached {
		klog.Warning("tag associated with tag category %s not attached to this resource or ancestor", TagCategoryRegion)
		return NewCheckError(MissingZoneRegions, fmt.Errorf("tag associated with tag category %s not attached to this resource or ancestor", TagCategoryRegion))
	}
	if !zoneTagAttached {
		klog.Warning("tag associated with tag category %s not attached to this resource or ancestor", TagCategoryZone)
		return NewCheckError(MissingZoneRegions, fmt.Errorf("tag associated with tag category %s not attached to this resource or ancestor", TagCategoryZone))
	}
	return nil
}

// CheckZoneTags will attempt to validate that the necessary tags are present to represent the
// various zones defined for a cluster.
func CheckZoneTags(ctx *CheckContext) *CheckError {
	klog.Info("Checking tags for multi-zone support.")

	// Get all failure domains
	klog.V(4).Info("Getting infrastructure configuration.")
	inf, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		klog.Errorf("Error getting infrastructure: %v", err)
		return NewCheckError(OpenshiftAPIError, err)
	}

	zoneCheckError := NewEmptyCheckErrorAggregator()

	// Perform check if FailureDomains defined.  We need 2 or more to require tags.
	klog.V(4).Info("Checking failure domains.")

	var fds []v1.VSpherePlatformFailureDomainSpec

	// In existing UPI clusters the VSphere field in the infra spec
	// may not be defined.
	if inf.Spec.PlatformSpec.VSphere != nil {
		fds = inf.Spec.PlatformSpec.VSphere.FailureDomains
	} else {
		klog.V(2).Infof("VSphere Infrastructure spec is empty, no FailureDomains configured. Skipping check.")
		return nil
	}

	if len(fds) > 1 {
		// Validate tags exist for cluster
		regionTagCategoryId, zoneTagCategoryId, err := validateTagCategories(ctx)
		if err != nil {
			return err
		}

		klog.V(4).Infof("Region: %s  Zone: %s", regionTagCategoryId, zoneTagCategoryId)

		// Iterate through each FailureDomain and check tags.
		for _, fd := range fds {

			// Validate compute cluster is defined correctly
			computeCluster := fd.Topology.ComputeCluster
			clusterPathRegexp := regexp.MustCompile(`^/(.*?)/host/(.*?)$`)
			clusterPathParts := clusterPathRegexp.FindStringSubmatch(computeCluster)

			if len(clusterPathParts) < 3 {
				klog.V(4).Info("Cluster parts are less than 3")
				zoneCheckError.addError(InvalidComputerClusterPath, fmt.Errorf("%s is missing full path to compute cluster", computeCluster))
			}
			computeClusterName := clusterPathParts[2]

			// Get DC first to initialize client
			klog.V(4).Info("Getting datacenter")
			datacenter, err := getDatacenter(ctx, fd.Topology.Datacenter)
			if err != nil {
				zoneCheckError.addError(FailedGettingDataCenter, fmt.Errorf("unable to get datacenter %s for failure domain %s: %s", fd.Topology.Datacenter, fd.Name, err))
				continue
			}

			// Get the ClusterComputeResource
			computeResourceMo, err := getClusterComputeResource(ctx, computeClusterName, datacenter)
			klog.V(4).Infof("ClusterComputeResource: %s", computeResourceMo)
			if err != nil {
				zoneCheckError.addError(FailedGettingComputeCluster, fmt.Errorf("unable to get ClusterComputeResource %s for failure domain %s: %s", computeClusterName, fd.Name, err))
				continue
			}

			validationCtx := validationContext{
				reference:           computeResourceMo.Reference(),
				regionTagCategoryID: regionTagCategoryId,
				zoneTagCategoryID:   zoneTagCategoryId,
			}

			// Validate tags for the current ComputeCluster
			errCheck := validateTagAttachment(ctx, validationCtx)
			if errCheck != nil {
				zoneCheckError.AddCheckError(errCheck)
				klog.Errorf("Multi-Zone support: ClusterComputeResource %s for failure domain %s: %s", computeClusterName, fd.Name, errCheck)
			}
		}
	} else {
		klog.V(2).Infof("No FailureDomains configured.  Skipping check.")
	}
	cerr := zoneCheckError.Join()
	if cerr != nil {
		fmt.Printf("We got something going here: %+v\n", cerr)
	}
	return cerr
}
