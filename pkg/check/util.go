package check

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	vim "github.com/vmware/govmomi/vim25/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/log"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
)

func getDatacenter(ctx *CheckContext, vCenter *VCenter, dcName string) (*object.Datacenter, error) {
	return vCenter.Cache.GetDatacenter(ctx.Context, dcName)
}

func getDataStoreByName(ctx *CheckContext, vCenter *VCenter, dsName string, dc *object.Datacenter) (*object.Datastore, error) {
	return vCenter.Cache.GetDatastore(ctx.Context, dc.Name(), dsName)
}

func getDatastoreByURL(ctx *CheckContext, dsURL string) (dsMo mo.Datastore, dcName string, vCenter *VCenter, err error) {
	for _, fd := range ctx.PlatformSpec.FailureDomains {
		// Get vCenter
		vCenter = ctx.VCenters[fd.Server]
		if vCenter == nil {
			err = fmt.Errorf("vCenter %v not found", fd.Server)
			return
		}

		// Lookup DS
		dsMo, err = vCenter.Cache.GetDatastoreByURL(ctx.Context, fd.Topology.Datacenter, dsURL)
		if err != nil {
			if err == cache.ErrDatastoreNotFound {
				continue
			}
			log.Logf("error fetching datastoreURL %s from datacenter %s", dsURL, fd.Topology.Datacenter)
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

func getDataStoreMoByName(ctx *CheckContext, vCenter *VCenter, datastoreName, dcName string) (mo.Datastore, error) {
	return vCenter.Cache.GetDatastoreMo(ctx.Context, dcName, datastoreName)
}

func getDatastore(ctx *CheckContext, vCenter *VCenter, dcName string, ref vim.ManagedObjectReference) (mo.Datastore, error) {
	return vCenter.Cache.GetDatastoreMoByReference(ctx.Context, dcName, ref)
}

func getComputeCluster(ctx *CheckContext, vCenter *VCenter, ref vim.ManagedObjectReference) (*mo.ClusterComputeResource, error) {
	var hostSystemMo mo.HostSystem
	var computeClusterMo mo.ClusterComputeResource
	pc := property.DefaultCollector(vCenter.VMClient)
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
func getResourcePool(ctx *CheckContext, vCenter *VCenter, ref vim.ManagedObjectReference) (*mo.ResourcePool, string, error) {
	var vmMo mo.VirtualMachine
	var resourcePoolMo mo.ResourcePool
	pc := property.DefaultCollector(vCenter.VMClient)
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

	resourcePoolPath, err := find.InventoryPath(tctx, vCenter.VMClient, resourcePoolMo.Reference())
	if err != nil {
		return nil, "", err
	}

	return &resourcePoolMo, resourcePoolPath, nil
}

// getClusterComputeResource returns the ComputeResource that matches the provided name.
func getClusterComputeResource(ctx *CheckContext, vCenter *VCenter, computeCluster string, datacenter *object.Datacenter) (*object.ClusterComputeResource, error) {
	klog.V(4).Infof("Looking for CC: %s", computeCluster)
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	finder := find.NewFinder(vCenter.VMClient)
	finder.SetDatacenter(datacenter)
	computeClusterMo, err := finder.ClusterComputeResource(tctx, computeCluster)
	if err != nil {
		clusters, err := finder.ComputeResourceList(ctx.Context, datacenter.InventoryPath)
		if err == nil {
			for _, cluster := range clusters {
				fmt.Printf("cluster: %s\n", cluster.InventoryPath)
			}
		}
		return nil, fmt.Errorf("unable to get cluster ComputeResource: %s", err)
	}

	return computeClusterMo, nil
}

// getCategories returns all tag categories.
func getCategories(ctx *CheckContext, vCenter *VCenter) ([]vapitags.Category, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	tagManager := vCenter.TagManager
	tags, err := tagManager.GetCategories(tctx)
	if err != nil {
		return nil, err
	}

	return tags, nil
}

func getHostsInHostGroup(ctx context.Context, computeCluster *object.ClusterComputeResource, failureDomain *ocpv1.VSpherePlatformFailureDomainSpec) ([]vim.ManagedObjectReference, error) {
	if failureDomain.ZoneAffinity == nil {
		return nil, nil
	}
	clusterConfigInfo, err := computeCluster.Configuration(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get cluster configuration. %v", err)
	}

	for _, group := range clusterConfigInfo.Group {
		if _, isType := group.(*vim.ClusterHostGroup); isType {
			hostGroup := group.(*vim.ClusterHostGroup)
			if hostGroup.Name != failureDomain.ZoneAffinity.HostGroup.HostGroup {
				continue
			}
			return hostGroup.Host, nil
		}
	}

	return nil, nil
}

// getAncestors returns a list of ancestor objects related to the passed in ManagedObjectReference.
func getAncestors(ctx *CheckContext, vctx validationContext) ([]mo.ManagedEntity, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	reference := vctx.reference

	defer cancel()

	ancestors, err := mo.Ancestors(tctx,
		vctx.vCenter.VMClient.RoundTripper,
		vctx.vCenter.VMClient.ServiceContent.PropertyCollector,
		reference)
	if err != nil {
		return nil, err
	}

	zoneAffinity := vctx.failureDomain.ZoneAffinity
	if zoneAffinity != nil && zoneAffinity.Type == ocpv1.HostGroupFailureDomainZone {
		computeCluster := vctx.computeCluster
		hosts, err := getHostsInHostGroup(ctx.Context, computeCluster, vctx.failureDomain)
		if err != nil {
			return nil, fmt.Errorf("unable to get hosts in host group: %v", err)
		}

		for _, host := range hosts {
			managedEntity := mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: host,
				},
			}
			ancestors = append(ancestors, managedEntity)
		}
	}

	return ancestors, err
}

func getAttachedTagsOnObjects(ctx *CheckContext, vctx *validationContext, referencesToCheck []mo.Reference) ([]vapitags.AttachedTags, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *util.Timeout)
	defer cancel()

	tagManager := vctx.vCenter.TagManager
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
		//   We only need to do this for legacy ini configs.  Yaml configs we expect all to be configured correctly.
		if checkContext.PlatformSpec != nil && checkContext.VMConfig.LegacyConfig != nil {
			if len(checkContext.PlatformSpec.VCenters) != 0 {
				// we need to check if we really need to add to VCenters and FailureDomains.
				vcenter := vCentersToMap(checkContext.PlatformSpec.VCenters)

				// vcenter is missing from the map, add it...
				for _, vCenter := range checkContext.VCenters {
					if _, ok := vcenter[vCenter.VCenterName]; !ok {
						klog.Warningf("vCenter %v is missing from vCenter map", vCenter.VCenterName)
						convertIntreeToPlatformSpec(config, vCenter.VCenterName, checkContext.PlatformSpec)
					}
				}

				// If platform spec defined vCenters, but no failure domains, this seems like invalid config.  We can
				// attempt to add failure domain as a failsafe, but only if legacy ini config was used.
				if len(checkContext.PlatformSpec.FailureDomains) == 0 {
					addFailureDomainsToPlatformSpec(config, checkContext.PlatformSpec, config.LegacyConfig.Workspace.VCenterIP)
				}
			} else {
				// If we are here, infrastructure resource hasn't been updated for any vCenter.  For multi vcenter support,
				// being here is not supported.  For 1 vCenter we will allow.
				if len(checkContext.VCenters) == 1 {
					for _, vCenter := range checkContext.VCenters {
						convertIntreeToPlatformSpec(config, vCenter.VCenterName, checkContext.PlatformSpec)
					}
				} else {
					klog.Error("infrastructure has not been configured correctly to support multiple vCenters.")
				}
			}
		}
	}
}

func convertIntreeToPlatformSpec(config *util.VSphereConfig, vcenter string, platformSpec *ocpv1.VSpherePlatformSpec) {
	// All this logic should only happen if using legacy cloud provider config and admin has not set up failure domain
	legacyCfg := config.LegacyConfig
	if ccmVcenter, ok := legacyCfg.VirtualCenter[legacyCfg.Workspace.VCenterIP]; ok {
		datacenters := strings.Split(ccmVcenter.Datacenters, ",")

		platformSpec.VCenters = append(platformSpec.VCenters, ocpv1.VSpherePlatformVCenterSpec{
			Server:      vcenter,
			Datacenters: datacenters,
		})
		addFailureDomainsToPlatformSpec(config, platformSpec, vcenter)
	}
}

func addFailureDomainsToPlatformSpec(config *util.VSphereConfig, platformSpec *ocpv1.VSpherePlatformSpec, vcenter string) {
	legacyCfg := config.LegacyConfig
	platformSpec.FailureDomains = append(platformSpec.FailureDomains, ocpv1.VSpherePlatformFailureDomainSpec{
		Name:   "",
		Region: "",
		Zone:   "",
		Server: vcenter,
		Topology: ocpv1.VSpherePlatformTopology{
			Datacenter:   legacyCfg.Workspace.Datacenter,
			Folder:       legacyCfg.Workspace.Folder,
			ResourcePool: legacyCfg.Workspace.ResourcePoolPath,
			Datastore:    legacyCfg.Workspace.DefaultDatastore,
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

func GetVCenter(checkContext *CheckContext, node *v1.Node) (*VCenter, error) {
	if checkContext.PlatformSpec != nil {
		// Determine which vCenter connection is in by looking at failure domain info.  If topology label is not found,
		// fall back and look for older beta version.
		region := node.Labels[v1.LabelTopologyRegion]
		if len(region) == 0 {
			region = node.Labels[v1.LabelFailureDomainBetaRegion]
		}
		zone := node.Labels[v1.LabelTopologyZone]
		if len(zone) == 0 {
			zone = node.Labels[v1.LabelFailureDomainBetaZone]
		}

		// In older clusters, the region/zone will be blank on nodes and there will be no FD.
		// However, region/zone being set is not enough to start looking for FailureDomain:
		// in case of single (auto-generated) FailureDomain, topology-awareness is not enabled
		// and we must fall back to the second "if" clause below (OCPBUGS-59319).
		if len(region) > 0 && len(zone) > 0 && len(checkContext.PlatformSpec.FailureDomains) > 1 {
			klog.V(2).Infof("Checking for region %v zone %v", region, zone)
			server := ""
			// Get failure domain
			for _, fd := range checkContext.PlatformSpec.FailureDomains {
				if strings.EqualFold(fd.Region, region) && strings.EqualFold(fd.Zone, zone) {
					server = fd.Server
					break
				}
			}

			// Maybe return error if server not found?
			if server == "" {
				return nil, fmt.Errorf("unable to determine vcenter for node %s", node.Name)
			}

			return checkContext.VCenters[server], nil
		} else {
			// Node is not configured w/ FD labels.  Admin may not have updated control plane after migrating to
			// multi vCenter config.
			klog.Warningf("failure domain labels missing from node %s", node.Name)
			if len(checkContext.PlatformSpec.FailureDomains) > 0 {
				klog.V(2).Infof("Returning first FD defined in platform spec.")
				for _, fd := range checkContext.PlatformSpec.FailureDomains {
					return checkContext.VCenters[fd.Server], nil
				}
			} else {
				klog.V(2).Infof("There are no failure domains in platform spec. Returning first vCenter from context.")
				for index := range checkContext.VCenters {
					return checkContext.VCenters[index], nil
				}
			}
		}
	} else {
		// Platform spec is not set.  This is old behavior before FDs.  Return only FD.
		klog.V(2).Infof("Infrastructure is not configured.  Returning first vCenter from context.")
		if len(checkContext.VCenters) > 1 {
			return nil, fmt.Errorf("invalid number of configured vCenters detected: %d.  Expected only 1.", len(checkContext.VCenters))
		}
		for index := range checkContext.VCenters {
			return checkContext.VCenters[index], nil
		}
	}

	return nil, fmt.Errorf("unable to determine vcenter for node %s", node.Name)
}
