package check

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	vim "github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
)

func getDatacenter(ctx *CheckContext, dcName string) (*object.Datacenter, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	finder := find.NewFinder(ctx.VMClient, false)
	dc, err := finder.Datacenter(tctx, dcName)
	if err != nil {
		return nil, fmt.Errorf("failed to access datacenter %s: %s", dcName, err)
	}
	return dc, nil
}

func getDataStoreByName(ctx *CheckContext, dsName string, dc *object.Datacenter) (*object.Datastore, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	finder := find.NewFinder(ctx.VMClient, false)
	finder.SetDatacenter(dc)
	ds, err := finder.Datastore(tctx, dsName)
	if err != nil {
		return nil, fmt.Errorf("failed to access datastore %s: %s", dsName, err)
	}
	return ds, nil
}

func getDatastore(ctx *CheckContext, ref vim.ManagedObjectReference) (mo.Datastore, error) {
	var dsMo mo.Datastore
	pc := property.DefaultCollector(ctx.VMClient)
	properties := []string{DatastoreInfoProperty, SummaryProperty}
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err := pc.RetrieveOne(tctx, ref, properties, &dsMo)
	if err != nil {
		return dsMo, fmt.Errorf("failed to get datastore object from managed reference: %v", err)
	}
	return dsMo, nil
}

func getComputeCluster(ctx *CheckContext, ref vim.ManagedObjectReference) (*mo.ClusterComputeResource, error) {
	var hostSystemMo mo.HostSystem
	var computeClusterMo mo.ClusterComputeResource
	pc := property.DefaultCollector(ctx.VMClient)
	properties := []string{"summary", "parent"}
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
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
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
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
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
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
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
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
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
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
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	tagManager := ctx.TagManager
	attachedTags, err := tagManager.GetAttachedTagsOnObjects(tctx, referencesToCheck)
	return attachedTags, err
}

// getVirtualMachine returns VirtualMachine based on provider ID passed in.  This will
// also load all properties related to VM using NodeProperties
func getVirtualMachine(ctx *CheckContext, dc *object.Datacenter, providerID string) (*mo.VirtualMachine, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	s := object.NewSearchIndex(ctx.VMClient)
	vmUUID := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(providerID, "vsphere://")))
	svm, err := s.FindByUuid(tctx, dc, vmUUID, true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM by UUID %s: %s", vmUUID, err)
	}
	if svm == nil {
		return nil, fmt.Errorf("unable to find VM by UUID %s", vmUUID)
	}

	// Load VM properties
	vm := object.NewVirtualMachine(ctx.VMClient, svm.Reference())

	var vmo mo.VirtualMachine
	err = vm.Properties(tctx, vm.Reference(), NodeProperties, &vmo)
	if err != nil {
		return nil, fmt.Errorf("failed to load properties for VM %s: %s", vmUUID, err)
	}

	return &vmo, nil
}
