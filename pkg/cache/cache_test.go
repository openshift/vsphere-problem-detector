package cache

import (
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
	"github.com/stretchr/testify/assert"
)

func TestCacheGetDatacenter(t *testing.T) {
	testSetup, cleanup, err := testlib.SetupSimulator(nil, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := NewCheckCache(testSetup.VMClient)

	// Populate the cache
	dcName := testSetup.VMConfig.Workspace.Datacenter
	dc, err := cache.GetDatacenter(testSetup.Context, dcName)
	assert.NoError(t, err)
	assert.Equal(t, dc.Name(), "DC0")

	// Shut down the simulated server
	cleanup()

	// Act: get cached datacenter, while the simulated vCenter is offline
	dc, err = cache.GetDatacenter(testSetup.Context, dcName)
	assert.NoError(t, err)
	assert.Equal(t, dc.Name(), "DC0")
}

func TestCacheGetDatastore(t *testing.T) {
	testSetup, cleanup, err := testlib.SetupSimulator(nil, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := NewCheckCache(testSetup.VMClient)

	// Populate the cache
	dcName := testSetup.VMConfig.Workspace.Datacenter
	dsName := testSetup.VMConfig.Workspace.DefaultDatastore
	ds, err := cache.GetDatastore(testSetup.Context, dcName, dsName)
	assert.NoError(t, err)
	assert.Equal(t, ds.Name(), "LocalDS_0")

	// Shut down the simulated server
	cleanup()

	// Act: get cached datastore, not used in the prev. call, while the simulated vCenter is offline
	ds, err = cache.GetDatastore(testSetup.Context, dcName, "LocalDS_3")
	assert.NoError(t, err)
	assert.Equal(t, ds.Name(), "LocalDS_3")
}

func TestCacheGetDatastoreByURL(t *testing.T) {
	testSetup, cleanup, err := testlib.SetupSimulator(nil, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := NewCheckCache(testSetup.VMClient)

	// Populate the cache
	dcName := testSetup.VMConfig.Workspace.Datacenter
	dsName := testSetup.VMConfig.Workspace.DefaultDatastore
	ds, err := cache.GetDatastore(testSetup.Context, dcName, dsName)
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_0", ds.Name())

	// Shut down the simulated server
	cleanup()

	// Act: get cached datastore, not used in the prev. call, while the simulated vCenter is offline
	mo, err := cache.GetDatastoreByURL(testSetup.Context, dcName, "testdata/default/govcsim-DC0-LocalDS_3-206027153")
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_3", mo.Info.GetDatastoreInfo().Name)
}

func TestCacheGetDatastoreMo(t *testing.T) {
	testSetup, cleanup, err := testlib.SetupSimulator(nil, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := NewCheckCache(testSetup.VMClient)

	// Populate the cache
	dcName := testSetup.VMConfig.Workspace.Datacenter
	dsName := testSetup.VMConfig.Workspace.DefaultDatastore
	ds, err := cache.GetDatastore(testSetup.Context, dcName, dsName)
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_0", ds.Name())

	// Shut down the simulated server
	cleanup()

	// Act: get cached datastore, not used in the prev. call, while the simulated vCenter is offline
	mo, err := cache.GetDatastoreMo(testSetup.Context, dcName, "LocalDS_3")
	assert.NoError(t, err)
	assert.Equal(t, "LocalDS_3", mo.Info.GetDatastoreInfo().Name)
}

func TestCacheGetStoragePods(t *testing.T) {
	testSetup, cleanup, err := testlib.SetupSimulator(nil, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("Failed to setup vSphere simulator: %s", err)
	}
	defer cleanup()

	cache := NewCheckCache(testSetup.VMClient)

	// Populate the cache
	initialPods, err := cache.GetStoragePods(testSetup.Context)
	assert.NoError(t, err)
	assert.Len(t, initialPods, 2)

	// Shut down the simulated server
	cleanup()

	// Act: get storage pods, while the simulated vCenter is offline
	finalPods, err := cache.GetStoragePods(testSetup.Context)
	assert.NoError(t, err)
	assert.Equal(t, initialPods, finalPods)
}
