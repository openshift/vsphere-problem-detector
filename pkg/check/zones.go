package check

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vim25types "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	v1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/log"
)

type validationContext struct {
	reference           vim25types.ManagedObjectReference
	regionTagCategoryID string
	zoneTagCategoryID   string
	zoneAffinity        *v1.VSphereFailureDomainZoneAffinity
	regionAffinity      *v1.VSphereFailureDomainRegionAffinity
	vCenter             *VCenter
	failureDomain       *v1.VSpherePlatformFailureDomainSpec
	computeCluster      *object.ClusterComputeResource
}

const (
	// TagCategoryRegion the tag category associated with regions.
	TagCategoryRegion = "openshift-region"

	// TagCategoryZone the tag category associated with zones.
	TagCategoryZone = "openshift-zone"
)

// validateTagCategories will verify that the categories exist for the region
// and zone tags.
func validateTagCategories(ctx *CheckContext, vCenter *VCenter) (string, string, error) {
	klog.V(4).Info("Validating tag categories...")
	categories, err := getCategories(ctx, vCenter)
	if err != nil {
		return "", "", err
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
		return "", "", fmt.Errorf("tag categories openshift-zone and openshift-region must be created")
	}
	return regionTagCategoryID, zoneTagCategoryID, nil
}

// validateTagAttachment will attempt to validate if region and zone tag is present
// in the ancestry of the ManagedObjectRefence that is provided in the
// validationContext.
func validateTagAttachment(ctx *CheckContext, vctx validationContext) error {
	klog.V(2).Infof("Validating tags for %s.", vctx.reference)
	referencesToCheck := []mo.Reference{vctx.reference}
	fd := vctx.failureDomain

	ancestors, err := getAncestors(ctx, vctx)
	if err != nil {
		klog.Error("Unable to get ancestors.")
		return err
	}

	for _, ancestor := range ancestors {
		referencesToCheck = append(referencesToCheck, ancestor.Reference())
	}
	attachedTags, err := getAttachedTagsOnObjects(ctx, &vctx, referencesToCheck)
	if err != nil {
		klog.Error("Unable to get attached tags.")
		return err
	}

	klog.V(4).Infof("Processing attached tags")
	regionTagAttached := false
	zoneTagAttached := false
	for _, attachedTag := range attachedTags {
		for _, tag := range attachedTag.Tags {
			klog.V(5).Infof("Current tag: %s", tag)
			if !regionTagAttached {
				if tag.CategoryID == vctx.regionTagCategoryID {
					if fd.RegionAffinity != nil && fd.RegionAffinity.Type == v1.DatacenterFailureDomainRegion {
						if attachedTag.ObjectID.Reference().Type == "Datacenter" {
							regionTagAttached = true
							klog.V(4).Infof("found region tag %s attached to datacenter(affinity == Datacenter): %v", tag.Name, attachedTag.ObjectID)
						}
					} else if fd.RegionAffinity != nil && fd.RegionAffinity.Type == v1.ComputeClusterFailureDomainRegion {
						if attachedTag.ObjectID.Reference().Type == "ClusterComputeResource" {
							regionTagAttached = true
							klog.V(4).Infof("found region tag %s attached to compute cluster(affinity == ComputeCluster): %v", tag.Name, attachedTag.ObjectID)
						}
					} else {
						regionTagAttached = true
						klog.V(4).Infof("found region tag attached to: %v", attachedTag.ObjectID)
					}
				}
			}
			if !zoneTagAttached {
				if tag.CategoryID == vctx.zoneTagCategoryID {
					if fd.ZoneAffinity != nil && fd.ZoneAffinity.Type == v1.ComputeClusterFailureDomainZone {
						if attachedTag.ObjectID.Reference().Type == "ClusterComputeResource" {
							zoneTagAttached = true
							klog.V(4).Infof("found zone tag %s attached to compute cluster(affinity == ComputeCluster): %v", tag.Name, attachedTag.ObjectID)
						}
					} else {
						zoneTagAttached = true
						klog.V(4).Infof("found zone tag attached to: %v", attachedTag.ObjectID)
					}
				}
			}
			if regionTagAttached && zoneTagAttached {
				return nil
			}
		}
	}

	klog.V(4).Infof("Region %t Zone %t", regionTagAttached, zoneTagAttached)
	var errs []string
	if !regionTagAttached {
		klog.Warning("Region not found")
		errs = append(errs, fmt.Sprintf("tag associated with tag category %s not attached to this resource or ancestor", TagCategoryRegion))
	}
	if !zoneTagAttached {
		klog.Warning("Zone not found")
		errs = append(errs, fmt.Sprintf("tag associated with tag category %s not attached to this resource or ancestor", TagCategoryZone))
	}
	return errors.New(strings.Join(errs, ","))
}

// CheckZoneTags will attempt to validate that the necessary tags are present to represent the
// various zones defined for a cluster.
func CheckZoneTags(ctx *CheckContext) error {
	klog.Info("Checking tags for multi-zone support.")
	var errs []error

	// Get all failure domains
	klog.V(4).Info("Getting infrastructure configuration.")
	inf, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		log.Logf("Error getting infrastructure: %v", err)
		return err
	}

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
		vCenterRegionCategories := make(map[string]string)
		vCenterZoneCategories := make(map[string]string)
		for _, vCenter := range ctx.VCenters {
			regionTagCategoryId, zoneTagCategoryId, err := validateTagCategories(ctx, vCenter)

			if err != nil {
				return fmt.Errorf("Multi-Zone support: %s", err)
			}
			klog.V(4).Infof("Region: %s  Zone: %s", regionTagCategoryId, zoneTagCategoryId)
			vCenterRegionCategories[vCenter.VCenterName] = regionTagCategoryId
			vCenterZoneCategories[vCenter.VCenterName] = zoneTagCategoryId
		}

		// Iterate through each FailureDomain and check tags.
		for _, fd := range fds {
			// Load vCenter for future use
			vCenter := ctx.VCenters[fd.Server]
			if vCenter == nil {
				return fmt.Errorf("Multi-Zone support: unable to check zone tags: vCenter %s for failure domain %s not found in cloud provider config", fd.Server, fd.Name)
			}

			// Validate compute cluster is defined correctly
			vsphereField := field.NewPath("platform").Child("vsphere")
			topologyField := vsphereField.Child("failureDomains").Child("topology")

			computeCluster := fd.Topology.ComputeCluster
			clusterPathRegexp := regexp.MustCompile(`^/(.*?)/host/(.*?)$`)
			clusterPathParts := clusterPathRegexp.FindStringSubmatch(computeCluster)
			if len(clusterPathParts) < 3 {
				klog.V(4).Info("Cluster parts are less than 3")
				errs = append(errs, field.Invalid(topologyField.Child("computeCluster"), computeCluster, "full path of cluster is required"))
			}
			computeClusterName := clusterPathParts[2]

			// Get DC first to initialize client
			klog.V(4).Info("Getting datacenter")
			datacenter, err := getDatacenter(ctx, vCenter, fd.Topology.Datacenter)
			if err != nil {
				errs = append(errs, fmt.Errorf("unable to get datacenter %s for failure domain %s: %s", fd.Topology.Datacenter, fd.Name, err))
				continue
			}

			// Get the ClusterComputeResource
			computeResourceMo, err := getClusterComputeResource(ctx, vCenter, computeClusterName, datacenter)
			klog.V(4).Infof("ClusterComputeResource: %s", computeResourceMo)
			if err != nil {
				errs = append(errs, fmt.Errorf("unable to get ClusterComputeResource %s for failure domain %s: %s", computeClusterName, fd.Name, err))
				continue
			}

			validationCtx := validationContext{
				reference:           computeResourceMo.Reference(),
				regionTagCategoryID: vCenterRegionCategories[fd.Server],
				regionAffinity:      fd.RegionAffinity,
				zoneAffinity:        fd.ZoneAffinity,
				vCenter:             ctx.VCenters[fd.Server],
				zoneTagCategoryID:   vCenterZoneCategories[fd.Server],
				failureDomain:       &fd,
				computeCluster:      computeResourceMo,
			}

			// Validate tags for the current ComputeCluster
			err = validateTagAttachment(ctx, validationCtx)
			if err != nil {
				errs = append(errs, fmt.Errorf("Multi-Zone support: ClusterComputeResource %s for failure domain %s: %s", computeClusterName, fd.Name, err))
			}
		}
	} else {
		klog.V(2).Infof("No FailureDomains configured.  Skipping check.")
	}

	if len(errs) > 0 {
		return join(errs)
	}
	return nil
}
