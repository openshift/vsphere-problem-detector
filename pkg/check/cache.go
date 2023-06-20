package check

import (
	"context"
	"fmt"
	"sync"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
)

// VSphereCache is an interface to cache of frequently accessed vCenter objects,
// such as datacenter or datastore.
type VSphereCache interface {
	// GetDatacenter returns datacenter by name.
	GetDatacenter(ctx context.Context, dcName string) (*object.Datacenter, error)
	// GetDataStore returns datastore by name or by path - both can be used.
	GetDatastore(ctx context.Context, dcName, dsName string) (*object.Datastore, error)
	// GetDatastoreByURL returns datastore by URL.
	GetDatastoreByURL(ctx context.Context, dcName, dsURL string) (mo.Datastore, error)
	// GetDataStoreMo returns datastore's ManagedObject by name or by path - both can be used.
	GetDatastoreMo(ctx context.Context, dcName, dsName string) (mo.Datastore, error)
	// GetDatastoreByURL returns datastore by URL.
	GetDatastoreMoByReference(ctx context.Context, dcName string, ref types.ManagedObjectReference) (mo.Datastore, error)
}

// vSphereCache caches frequently accessed vCenter objects for a single check run.
type vSphereCache struct {
	vmClient    *vim25.Client
	datacenters map[string]*cachedDatacenter
	mutex       sync.Mutex
}

type cachedDatacenter struct {
	dc         *object.Datacenter
	dcPath     string
	datastores []*cachedDatastore
}

type cachedDatastore struct {
	dsPath string
	ds     *object.Datastore
	dsMo   *mo.Datastore
}

var _ VSphereCache = &vSphereCache{}

func NewCheckCache(vmClient *vim25.Client) VSphereCache {
	return &vSphereCache{
		vmClient:    vmClient,
		datacenters: make(map[string]*cachedDatacenter),
	}
}

func (c *vSphereCache) getDatacenterLocked(ctx context.Context, dcName string) (*cachedDatacenter, error) {
	if cdc, found := c.datacenters[dcName]; found {
		klog.V(4).Infof("Datacenter %s found in cache", dcName)
		return cdc, nil
	}

	klog.V(4).Infof("Loading datacenter %s", dcName)
	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()
	finder := find.NewFinder(c.vmClient, false)
	dcObject, err := finder.Datacenter(tctx, dcName)
	if err != nil {
		return nil, fmt.Errorf("failed to access datacenter %s: %s", dcName, err)
	}

	cdc := &cachedDatacenter{
		dc:         dcObject,
		dcPath:     dcObject.InventoryPath,
		datastores: nil,
	}
	c.datacenters[dcName] = cdc
	return cdc, nil
}

func (c *vSphereCache) getDatastoresLocked(ctx context.Context, dcName string) ([]*cachedDatastore, error) {
	cdc, err := c.getDatacenterLocked(ctx, dcName)
	if err != nil {
		return nil, err
	}
	if cdc.datastores != nil {
		klog.V(4).Infof("Datastores for datacenter %s found in cache", dcName)
		return cdc.datastores, nil
	}

	klog.V(4).Infof("Loading datastores for datacenter %s", dcName)
	// Retrieve both object.Datastore and ManagedObjects
	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()

	finder := find.NewFinder(c.vmClient, false)
	finder.SetDatacenter(cdc.dc)
	datastores, err := finder.DatastoreList(tctx, "*")
	if err != nil {
		klog.Errorf("failed to get all the datastores. err: %+v", err)
		return nil, err
	}

	var dsList []types.ManagedObjectReference
	for _, ds := range datastores {
		dsList = append(dsList, ds.Reference())
	}

	var dsMoList []mo.Datastore
	pc := property.DefaultCollector(cdc.dc.Client())
	properties := []string{DatastoreInfoProperty, SummaryProperty, "customValue"}
	err = pc.Retrieve(tctx, dsList, properties, &dsMoList)
	if err != nil {
		klog.Errorf("failed to get Datastore managed objects from datastore objects."+
			" dsObjList: %+v, properties: %+v, err: %v", dsList, properties, err)
		return nil, err
	}

	var cachedDataStores []*cachedDatastore
	var errs []error
	// match datastores and dsMoList together
	for _, ds := range datastores {
		moFound := false
		for _, mo := range dsMoList {
			moName := mo.Info.GetDatastoreInfo().Name
			dsName := ds.Name()
			if moName == dsName {
				cds := &cachedDatastore{
					ds:     ds,
					dsMo:   &mo,
					dsPath: ds.InventoryPath,
				}
				cachedDataStores = append(cachedDataStores, cds)
				moFound = true
				break
			}
		}
		if !moFound {
			errs = append(errs, fmt.Errorf("cannot find managed object for datastore %s", ds.Name()))
		}
	}
	if errs != nil {
		return nil, errors.NewAggregate(errs)
	}
	cdc.datastores = cachedDataStores
	return cachedDataStores, nil
}

func (c *vSphereCache) GetDatacenter(ctx context.Context, dcName string) (*object.Datacenter, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cdc, err := c.getDatacenterLocked(ctx, dcName)
	if err != nil {
		return nil, err
	}
	return cdc.dc, nil
}

func (c *vSphereCache) GetDatastore(ctx context.Context, dcName, dsName string) (*object.Datastore, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cdss, err := c.getDatastoresLocked(ctx, dcName)
	if err != nil {
		return nil, err
	}
	for _, cds := range cdss {
		// some callers use name, others use path
		if cds.ds.Name() == dsName || cds.dsPath == dsName {
			return cds.ds, nil
		}
	}
	return nil, fmt.Errorf("cannot find datastore %s in datacenter %s", dsName, dcName)
}

func (c *vSphereCache) GetDatastoreByURL(ctx context.Context, dcName, dsURL string) (mo.Datastore, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cdss, err := c.getDatastoresLocked(ctx, dcName)
	if err != nil {
		return mo.Datastore{}, err
	}

	for _, cds := range cdss {
		if cds.dsMo.Info.GetDatastoreInfo().Url == dsURL {
			klog.V(4).Infof("Found datastore MoRef %v for datastoreURL: %q in datacenter: %q",
				cds.dsMo.Reference(), dsURL, dcName)
			return *cds.dsMo, nil
		}
	}
	err = fmt.Errorf("couldn't find Datastore given URL %q in datacenter %s", dsURL, dcName)
	klog.Error(err)
	return mo.Datastore{}, err
}

func (c *vSphereCache) GetDatastoreMo(ctx context.Context, dcName, dsName string) (mo.Datastore, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cdss, err := c.getDatastoresLocked(ctx, dcName)
	if err != nil {
		return mo.Datastore{}, err
	}

	for _, cds := range cdss {
		// some callers use name, others use path
		if cds.ds.Name() == dsName || cds.dsPath == dsName {
			return *cds.dsMo, nil
		}
	}
	err = fmt.Errorf("couldn't find Datastore named %q in datacenter %s", dsName, dcName)
	klog.Error(err)
	return mo.Datastore{}, err
}

func (c *vSphereCache) GetDatastoreMoByReference(ctx context.Context, dcName string, ref types.ManagedObjectReference) (mo.Datastore, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cdss, err := c.getDatastoresLocked(ctx, dcName)
	if err != nil {
		return mo.Datastore{}, err
	}

	for _, cds := range cdss {
		// some callers use name, others use path
		if cds.ds.Reference() == ref {
			return *cds.dsMo, nil
		}
	}
	err = fmt.Errorf("couldn't find Datastore with reference %+v in datacenter %s", ref, dcName)
	klog.Error(err)
	return mo.Datastore{}, err
}
