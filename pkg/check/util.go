package check

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	vim "github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/vsphere"
)

func getDatacenter(ctx *CheckContext, dcName string) (*object.Datacenter, error) {
	return ctx.Cache.GetDatacenter(ctx.Context, dcName)
}

func getDataStoreByName(ctx *CheckContext, dsName string, dc *object.Datacenter) (*object.Datastore, error) {
	return ctx.Cache.GetDatastore(ctx.Context, dc.Name(), dsName)
}

func getDatastoreByURL(ctx *CheckContext, dsURL string) (dsMo mo.Datastore, dcName string, err error) {
	for _, fd := range ctx.PlatformSpec.FailureDomains {
		dsMo, err = ctx.Cache.GetDatastoreByURL(ctx.Context, fd.Topology.Datacenter, dsURL)
		if err != nil {
			if err == cache.ErrDatastoreNotFound {
				continue
			}
			klog.Errorf("error fetching datastoreURL %s from datacenter %s", dsURL, fd.Topology.Datacenter)
			return
		}
		if dsMo.Info.GetDatastoreInfo().Url == dsURL {
			dcName = fd.Topology.Datacenter
			return
		}
	}

	err = fmt.Errorf("unable to find datastore with URL %s", dsURL)
	return
}

func getDataStoreMoByName(ctx *CheckContext, datastoreName, dcName string) (mo.Datastore, error) {
	return ctx.Cache.GetDatastoreMo(ctx.Context, dcName, datastoreName)
}

func getDatastore(ctx *CheckContext, dcName string, ref vim.ManagedObjectReference) (mo.Datastore, error) {
	return ctx.Cache.GetDatastoreMoByReference(ctx.Context, dcName, ref)
}

func getComputeCluster(ctx *CheckContext, ref vim.ManagedObjectReference) (*mo.ClusterComputeResource, error) {
	var hostSystemMo mo.HostSystem
	var computeClusterMo mo.ClusterComputeResource
	pc := property.DefaultCollector(ctx.VMClient)
	properties := []string{"summary", "parent"}
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	err := pc.RetrieveOne(tctx, ref, properties, &hostSystemMo)
	if err != nil {
		return nil, fmt.Errorf("failed to get host system object from managed reference: %v", err)
	}

	if hostSystemMo.Parent.Type == "ClusterComputeResource" {
		err := pc.RetrieveOne(tctx, hostSystemMo.Parent.Reference(), []string{}, &computeClusterMo)
		if err != nil {
			return nil, fmt.Errorf("failed to get compute cluster resource object from managed reference: %v", err)
		}
		return &computeClusterMo, nil
	}
	return nil, errors.New("compute cluster resource not associated with managed reference")
}

// getResourcePool returns the parent resource pool for a given Virtual Machine. If the parent is a VirtualApp,
// then the ResourcePool owner of that VirtualApp is returned instead. Additionally, the Inventory Path of the
// Resource Pool is returned since the unpathed name alone is often not unique.
func getResourcePool(ctx *CheckContext, ref vim.ManagedObjectReference) (*mo.ResourcePool, string, error) {
	var vmMo mo.VirtualMachine
	var resourcePoolMo mo.ResourcePool
	pc := property.DefaultCollector(ctx.VMClient)
	properties := []string{"resourcePool"}
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	err := pc.RetrieveOne(tctx, ref, properties, &vmMo)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get virtual machine object from managed reference: %v", err)
	}

	err = pc.RetrieveOne(tctx, vmMo.ResourcePool.Reference(), []string{}, &resourcePoolMo)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get resource pool object from managed reference: %v", err)
	}

	if resourcePoolMo.Reference().Type == "VirtualApp" {
		err = pc.RetrieveOne(tctx, resourcePoolMo.Parent.Reference(), []string{}, &resourcePoolMo)
	}

	resourcePoolPath, err := find.InventoryPath(tctx, ctx.VMClient, resourcePoolMo.Reference())
	if err != nil {
		return nil, "", err
	}

	return &resourcePoolMo, resourcePoolPath, nil
}

// getClusterComputeResource returns the ComputeResource that matches the provided name.
func getClusterComputeResource(ctx *CheckContext, computeCluster string, datacenter *object.Datacenter) (*object.ClusterComputeResource, error) {
	klog.V(4).Infof("Looking for CC: %s", computeCluster)
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	finder := find.NewFinder(ctx.VMClient)
	finder.SetDatacenter(datacenter)
	computeClusterMo, err := finder.ClusterComputeResource(tctx, computeCluster)
	if err != nil {
		klog.Errorf("Unable to get cluster ComputeResource: %s", err)
	}
	return computeClusterMo, err
}

// getCategories returns all tag categories.
func getCategories(ctx *CheckContext) ([]vapitags.Category, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	tagManager := ctx.TagManager
	tags, err := tagManager.GetCategories(tctx)
	if err != nil {
		return nil, err
	}

	return tags, nil
}

// getAncestors returns a list of ancestor objects related to the passed in ManagedObjectReference.
func getAncestors(ctx *CheckContext, reference vim.ManagedObjectReference) ([]mo.ManagedEntity, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	ancestors, err := mo.Ancestors(tctx,
		ctx.VMClient.RoundTripper,
		ctx.VMClient.ServiceContent.PropertyCollector,
		reference)
	if err != nil {
		return nil, err
	}
	return ancestors, err
}

func getAttachedTagsOnObjects(ctx *CheckContext, referencesToCheck []mo.Reference) ([]vapitags.AttachedTags, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	tagManager := ctx.TagManager
	attachedTags, err := tagManager.GetAttachedTagsOnObjects(tctx, referencesToCheck)
	return attachedTags, err
}

func ConvertToPlatformSpec(infra *ocpv1.Infrastructure, checkContext *CheckContext) {
	checkContext.PlatformSpec = &ocpv1.VSpherePlatformSpec{}

	if infra.Spec.PlatformSpec.VSphere != nil {
		infra.Spec.PlatformSpec.VSphere.DeepCopyInto(checkContext.PlatformSpec)
	}

	if checkContext.VMConfig != nil {
		config := checkContext.VMConfig
		if checkContext.PlatformSpec != nil {
			if len(checkContext.PlatformSpec.VCenters) != 0 {
				// we need to check if we really need to add to VCenters and FailureDomains
				vcenter := vCentersToMap(checkContext.PlatformSpec.VCenters)

				// vcenter is missing from the map, add it...
				if _, ok := vcenter[config.Workspace.VCenterIP]; !ok {
					convertIntreeToPlatformSpec(config, checkContext.PlatformSpec)
				}

				if len(checkContext.PlatformSpec.FailureDomains) == 0 {
					addFailureDomainsToPlatformSpec(config, checkContext.PlatformSpec)
				}
			} else {
				convertIntreeToPlatformSpec(config, checkContext.PlatformSpec)
			}
		}
	}
}

func convertIntreeToPlatformSpec(config *vsphere.VSphereConfig, platformSpec *ocpv1.VSpherePlatformSpec) {
	if ccmVcenter, ok := config.VirtualCenter[config.Workspace.VCenterIP]; ok {
		datacenters := strings.Split(ccmVcenter.Datacenters, ",")

		platformSpec.VCenters = append(platformSpec.VCenters, ocpv1.VSpherePlatformVCenterSpec{
			Server:      config.Workspace.VCenterIP,
			Datacenters: datacenters,
		})
		addFailureDomainsToPlatformSpec(config, platformSpec)
	}
}

func addFailureDomainsToPlatformSpec(config *vsphere.VSphereConfig, platformSpec *ocpv1.VSpherePlatformSpec) {
	platformSpec.FailureDomains = append(platformSpec.FailureDomains, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "",
		Region: "",
		Zone:   "",
		Server: config.Workspace.VCenterIP,
		Topology: ocpv1.VSpherePlatformTopology{
			Datacenter:   config.Workspace.Datacenter,
			Folder:       config.Workspace.Folder,
			ResourcePool: config.Workspace.ResourcePoolPath,
			Datastore:    config.Workspace.DefaultDatastore,
		},
	})
}

func vCentersToMap(vcenters []ocpv1.VSpherePlatformVCenterSpec) map[string]ocpv1.VSpherePlatformVCenterSpec {
	vcenterMap := make(map[string]ocpv1.VSpherePlatformVCenterSpec, len(vcenters))
	for _, v := range vcenters {
		vcenterMap[v.Server] = v
	}
	return vcenterMap
}
