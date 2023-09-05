package check

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi/pbm"
	"github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	vim "github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
)

const (
	dsParameter            = "datastore"
	storagePolicyParameter = "storagepolicyname"
	dataStoreURL           = "datastoreurl"
	// Maximum length of <cluster-id>-dynamic-pvc-<uuid> for volume names.
	// Kubernetes uses 90, https://github.com/kubernetes/kubernetes/blob/93d288e2a47fa6d497b50d37c8b3a04e91da4228/pkg/volume/vsphere_volume/vsphere_volume_util.go#L100
	// Using 63 to work around https://bugzilla.redhat.com/show_bug.cgi?id=1926943
	maxVolumeName         = 63
	dataCenterType        = "Datacenter"
	DatastoreInfoProperty = "info"
	SummaryProperty       = "summary"

	inTreeProvisionerName = "kubernetes.io/vsphere-volume"
	csiProvisionerName    = "csi.vsphere.vmware.com"

	dataStoreType = "type"
)

var (
	dataStoreTypesMetric = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Name:           "vsphere_datastore_total",
			Help:           "Number of DataStores used by the cluster.",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{dataStoreType},
	)
)

func init() {
	legacyregistry.MustRegister(dataStoreTypesMetric)
}

// dataStoreTypeCollector collects types of each datastore inspected by CheckStorageClasses
// for metrics.
type dataStoreTypeCollector map[string]string

func (d dataStoreTypeCollector) addDataStore(name, dsType string) {
	if d == nil {
		return
	}
	// Make the datastore type case insensitive, just in case vCenter API changes.
	dsType = strings.ToLower(dsType)
	d[name] = dsType
}

func (d dataStoreTypeCollector) getDataStoreTypeCount() map[string]int {
	if d == nil {
		return nil
	}

	m := make(map[string]int)
	for _, dsType := range d {
		m[dsType]++
	}
	return m
}

// CheckStorageClasses tests that datastore name in all StorageClasses in the cluster is short enough.
func CheckStorageClasses(ctx *CheckContext) *CheckError {
	// reset the metric so as if types have changed we don't emit them again
	dataStoreTypesMetric.Reset()

	infra, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		return NewCheckError(OpenshiftAPIError, err)
	}

	scs, err := ctx.KubeClient.ListStorageClasses(ctx.Context)
	if err != nil {
		return NewCheckError(OpenshiftAPIError, err)
	}

	scCheckError := NewEmptyCheckErrorAggregator()
	dsTypes := make(dataStoreTypeCollector)
	for i := range scs {
		sc := scs[i]
		if sc.Provisioner != inTreeProvisionerName &&
			sc.Provisioner != csiProvisionerName {
			klog.V(4).Infof("Skipping storage class %s: not a vSphere class", sc.Name)
			continue
		}

		for k, v := range sc.Parameters {
			switch strings.ToLower(k) {
			case dsParameter:
				if err := checkDataStore(ctx, v, dsTypes); err != nil {
					klog.V(2).Infof("CheckStorageClasses: %s: %s", sc.Name, err)
					scCheckError.AddCheckError(err)
				}
			case dataStoreURL:
				if err := checkDataStoreWithURL(ctx, v, dsTypes); err != nil {
					klog.V(2).Infof("Checking storageclass %s: %v", sc.Name, err)
					scCheckError.AddCheckError(err)
				}
			case storagePolicyParameter:
				if err := checkStoragePolicy(ctx, v, infra, dsTypes); err != nil {
					klog.V(2).Infof("CheckStorageClasses: %s: %s", sc.Name, err)
					scCheckError.AddCheckError(err)
				}
			default:
				// There is neither datastore: nor storagepolicyname: in the StorageClass,
				// check the default datastore and collect its type.
				if err := checkDefaultDatastoreWithDSType(ctx, dsTypes); err != nil {
					klog.V(2).Infof("CheckStorageClasses: %s: %s", sc.Name, err)
				}
			}
		}
	}

	for dsType, count := range dsTypes.getDataStoreTypeCount() {
		dataStoreTypesMetric.WithLabelValues(dsType).Set(float64(count))
	}

	klog.V(2).Infof("CheckStorageClasses checked %d storage classes, %d problems found", len(scs), len(scCheckError.ErrorItems))
	return scCheckError.Join()
}

// CheckDefaultDatastore checks that the default data store name in vSphere config file is short enough.
func CheckDefaultDatastore(ctx *CheckContext) *CheckError {
	return checkDefaultDatastoreWithDSType(ctx, nil)
}

func checkDefaultDatastoreWithDSType(ctx *CheckContext, dsTypes dataStoreTypeCollector) *CheckError {
	dsName := ctx.VMConfig.Workspace.DefaultDatastore
	if err := checkDataStore(ctx, dsName, dsTypes); err != nil {
		klog.Errorf("defaultDatastore %q in vSphere configuration: %s", dsName, err)
		return err
	}
	return nil
}

// checkStoragePolicy lists all compatible datastores and checks their names are short.
func checkStoragePolicy(ctx *CheckContext, policyName string, infrastructure *configv1.Infrastructure, dsTypes dataStoreTypeCollector) *CheckError {
	klog.V(4).Infof("Checking storage policy %s", policyName)

	pbm, err := getPolicy(ctx, policyName)
	if err != nil {
		return NewCheckError(FailedGettingStoragePolicy, err)
	}

	if len(pbm) == 0 {
		return NewCheckError(StoragePolicyNotFound, fmt.Errorf("error listing storage policy %s: policy not found", policyName))
	}

	if len(pbm) > 1 {
		return NewCheckError(FailedListingStoragePolicy, fmt.Errorf("error listing storage policy %s: multiple (%d) policies found", policyName, len(pbm)))
	}

	dataStores, err := getPolicyDatastores(ctx, pbm[0].GetPbmProfile().ProfileId)
	if err != nil {
		klog.V(2).Infof("unable to list policy datastores: %v", err)
		// we may not have sufficient permission to list all datastores and hence we can ignore the check
		return nil
	}
	klog.V(4).Infof("Policy %q is compatible with datastores %v", policyName, dataStores)

	policyCheckError := NewEmptyCheckErrorAggregator()
	for _, dataStore := range dataStores {
		err := checkDataStore(ctx, dataStore, dsTypes)
		if err != nil {
			policyCheckError.AddCheckError(err)
		}
	}
	return policyCheckError.Join()
}

// checkStoragePolicy lists all datastores compatible with given policy.
func getPolicyDatastores(ctx *CheckContext, profileID types.PbmProfileId) ([]string, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	pbmClient, err := pbm.NewClient(tctx, ctx.VMClient)
	if err != nil {
		return nil, fmt.Errorf("getPolicyDatastores: error creating pbm client: %v", err)
	}

	// Load all datastores in vSphere
	kind := []string{"Datastore"}
	m := view.NewManager(ctx.VMClient)

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	v, err := m.CreateContainerView(tctx, ctx.VMClient.ServiceContent.RootFolder, kind, true)
	if err != nil {
		return nil, fmt.Errorf("getPolicyDatastores: error creating container view: %v", err)
	}

	var content []vim.ObjectContent
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err = v.Retrieve(tctx, kind, []string{"name"}, &content)
	_ = v.Destroy(tctx)
	if err != nil {
		return nil, fmt.Errorf("getPolicyDatastores: error listing datastores: %v", err)
	}

	// Store the datastores in this map HubID -> DatastoreName
	datastoreNames := make(map[string]string)
	var hubs []types.PbmPlacementHub

	for _, ds := range content {
		hubs = append(hubs, types.PbmPlacementHub{
			HubType: ds.Obj.Type,
			HubId:   ds.Obj.Value,
		})
		datastoreNames[ds.Obj.Value] = ds.PropSet[0].Val.(string)
	}

	req := []types.BasePbmPlacementRequirement{
		&types.PbmPlacementCapabilityProfileRequirement{
			ProfileId: profileID,
		},
	}

	// Match the datastores with the policy
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	res, err := pbmClient.CheckRequirements(tctx, hubs, nil, req)
	if err != nil {
		return nil, fmt.Errorf("getPolicyDatastores: error fetching matching datastores: %v", err)
	}

	var dataStores []string
	for _, hub := range res.CompatibleDatastores() {
		datastoreName := datastoreNames[hub.HubId]
		dataStores = append(dataStores, datastoreName)
	}
	return dataStores, nil
}

func getPolicy(ctx *CheckContext, name string) ([]types.BasePbmProfile, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	c, err := pbm.NewClient(tctx, ctx.VMClient)
	if err != nil {
		return nil, fmt.Errorf("error creating pbm client: %v", err)
	}
	rtype := types.PbmProfileResourceType{
		ResourceType: string(types.PbmProfileResourceTypeEnumSTORAGE),
	}
	category := types.PbmProfileCategoryEnumREQUIREMENT

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	ids, err := c.QueryProfile(tctx, rtype, string(category))
	if err != nil {
		return nil, fmt.Errorf("error querying storage profiles: %v", err)
	}

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	profiles, err := c.RetrieveContent(tctx, ids)
	if err != nil {
		return nil, fmt.Errorf("error retrieving detailed storage profiles: %v", err)
	}

	for _, p := range profiles {
		if p.GetPbmProfile().Name == name {
			return []types.BasePbmProfile{p}, nil
		}
	}
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	profileContent, err := c.RetrieveContent(tctx, []types.PbmProfileId{{UniqueId: name}})
	if err != nil {
		return nil, fmt.Errorf("error getting pbm profiles: %v", err)
	}
	return profileContent, nil
}

func checkDataStore(ctx *CheckContext, dsName string, dsTypes dataStoreTypeCollector) *CheckError {
	klog.V(2).Infof("checking datastore %s for permissions", dsName)

	dsMo, err := getDataStoreMoByName(ctx, dsName)
	if err != nil {
		return NewCheckError(FailedGettingDatastore, err)
	}
	dataStoreCheckError := NewEmptyCheckErrorAggregator()

	if err := checkForDatastoreCluster(ctx, dsMo, dsName, dsTypes); err != nil {
		dataStoreCheckError.AddCheckError(err)
	}
	if err := checkDatastorePrivileges(ctx, dsName, dsMo.Reference()); err != nil {
		dataStoreCheckError.AddCheckError(err)
	}
	return dataStoreCheckError.Join()
}

func checkDataStoreWithURL(ctx *CheckContext, dsURL string, dsTypes dataStoreTypeCollector) *CheckError {
	klog.V(2).Infof("checking datastore %s for permissions", dsURL)
	dsMo, err := getDatastoreByURL(ctx, dsURL)
	if err != nil {
		return NewCheckError(FailedGettingDatastore, err)
	}

	errCheck := NewEmptyCheckErrorAggregator()

	if err := checkForDatastoreCluster(ctx, dsMo, dsURL, dsTypes); err != nil {
		errCheck.AddCheckError(err)
	}
	if err := checkDatastorePrivileges(ctx, dsURL, dsMo.Reference()); err != nil {
		errCheck.AddCheckError(err)
	}
	return errCheck.Join()
}

func checkForDatastoreCluster(ctx *CheckContext, dsMo mo.Datastore, dataStoreName string, dsTypes dataStoreTypeCollector) *CheckError {
	// Collect DS type
	dsType := dsMo.Summary.Type
	klog.V(4).Infof("Datastore %s is of type %s", dataStoreName, dsType)
	dsTypes.addDataStore(dataStoreName, dsType)

	content, err := ctx.Cache.GetStoragePods(ctx.Context)
	if err != nil {
		return NewCheckError(FailedGettingDatastoreCluster, err)
	}
	for _, ds := range content {
		for _, child := range ds.Folder.ChildEntity {
			tDS, err := getDatastore(ctx, child)
			if err != nil {
				// we may not have permissions to fetch unrelated datastores in OCP
				// and hence we are going to ignore the error.
				klog.Errorf("fetching datastore %s failed: %v", child.String(), err)
				continue
			}
			if tDS.Summary.Url == dsMo.Summary.Url {
				return NewCheckError(DatastoreClusterInUse, fmt.Errorf("datastore %s is part of %s datastore cluster", tDS.Summary.Name, ds.Summary.Name))
			}
		}
	}
	klog.V(4).Infof("Checked datastore %s for SRDS - no problems found", dataStoreName)
	return nil
}
