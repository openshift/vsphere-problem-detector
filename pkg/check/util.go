package check

import (
	"context"
	"errors"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	vim "github.com/vmware/govmomi/vim25/types"
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
