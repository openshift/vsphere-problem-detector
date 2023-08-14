package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCacheGetDatacenter(t *testing.T) {
	ctx, cleanup, err := setupSimulator(nil, defaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := ctx.Cache

	// Populate the cache
	dcName := ctx.VMConfig.Workspace.Datacenter
	dc, err := cache.GetDatacenter(ctx.Context, dcName)
	assert.NoError(t, err)
	assert.Equal(t, dc.Name(), "DC0")

	// Shut down the simulated server
	cleanup()

	// Act: get cached datacenter, while the simulated vCenter is offline
	dc, err = cache.GetDatacenter(ctx.Context, dcName)
	assert.NoError(t, err)
	assert.Equal(t, dc.Name(), "DC0")
}

func TestCacheGetDatastore(t *testing.T) {
	ctx, cleanup, err := setupSimulator(nil, defaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := ctx.Cache

	// Populate the cache
	dcName := ctx.VMConfig.Workspace.Datacenter
	dsName := ctx.VMConfig.Workspace.DefaultDatastore
	ds, err := cache.GetDatastore(ctx.Context, dcName, dsName)
	assert.NoError(t, err)
	assert.Equal(t, ds.Name(), "LocalDS_0")

	// Shut down the simulated server
	cleanup()

	// Act: get cached datastore, not used in the prev. call, while the simulated vCenter is offline
	ds, err = cache.GetDatastore(ctx.Context, dcName, "LocalDS_3")
	assert.NoError(t, err)
	assert.Equal(t, ds.Name(), "LocalDS_3")
}

func TestCacheGetDatastoreByURL(t *testing.T) {
	ctx, cleanup, err := setupSimulator(nil, defaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := ctx.Cache

	// Populate the cache
	dcName := ctx.VMConfig.Workspace.Datacenter
	dsName := ctx.VMConfig.Workspace.DefaultDatastore
	ds, err := cache.GetDatastore(ctx.Context, dcName, dsName)
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_0", ds.Name())

	// Shut down the simulated server
	cleanup()

	// Act: get cached datastore, not used in the prev. call, while the simulated vCenter is offline
	mo, err := cache.GetDatastoreByURL(ctx.Context, dcName, "testdata/default/govcsim-DC0-LocalDS_3-206027153")
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_3", mo.Info.GetDatastoreInfo().Name)
}

func TestCacheGetDatastoreMo(t *testing.T) {
	ctx, cleanup, err := setupSimulator(nil, defaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := ctx.Cache

	// Populate the cache
	dcName := ctx.VMConfig.Workspace.Datacenter
	dsName := ctx.VMConfig.Workspace.DefaultDatastore
	ds, err := cache.GetDatastore(ctx.Context, dcName, dsName)
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_0", ds.Name())

	// Shut down the simulated server
	cleanup()

	// Act: get cached datastore, not used in the prev. call, while the simulated vCenter is offline
	mo, err := cache.GetDatastoreMo(ctx.Context, dcName, "LocalDS_3")
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_3", mo.Info.GetDatastoreInfo().Name)
}

func TestCacheGetStoragePods(t *testing.T) {
	ctx, cleanup, err := setupSimulator(nil, defaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := ctx.Cache

	// Populate the cache
	initialPods, err := cache.GetStoragePods(ctx.Context)
	assert.NoError(t, err)
	assert.Len(t, initialPods, 2)

	// Shut down the simulated server
	cleanup()

	// Act: get storage pods, while the simulated vCenter is offline
	finalPods, err := cache.GetStoragePods(ctx.Context)
	assert.NoError(t, err)
	assert.Equal(t, initialPods, finalPods)
}
