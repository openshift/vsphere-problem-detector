package check

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/vmware/govmomi/property"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi/find"
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
	// Maximum length of <cluster-id>-dynamic-pvc-<uuid> for volume names.
	// Kubernetes uses 90, https://github.com/kubernetes/kubernetes/blob/93d288e2a47fa6d497b50d37c8b3a04e91da4228/pkg/volume/vsphere_volume/vsphere_volume_util.go#L100
	// Using 63 to work around https://bugzilla.redhat.com/show_bug.cgi?id=1926943
	maxVolumeName         = 63
	dataCenterType        = "Datacenter"
	DatastoreInfoProperty = "info"
	SummaryProperty       = "summary"
)

// CheckStorageClasses tests that datastore name in all StorageClasses in the cluster is short enough.
func CheckStorageClasses(ctx *CheckContext) error {
	infra, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		return err
	}

	scs, err := ctx.KubeClient.ListStorageClasses(ctx.Context)
	if err != nil {
		return err
	}

	var errs []error
	for i := range scs {
		sc := &scs[i]
		if sc.Provisioner != "kubernetes.io/vsphere-volume" {
			klog.V(4).Infof("Skipping storage class %s: not a vSphere class", sc.Name)
			continue
		}

		for k, v := range sc.Parameters {
			switch strings.ToLower(k) {
			case dsParameter:
				if err := checkDataStore(ctx, v, infra); err != nil {
					klog.V(2).Infof("CheckStorageClasses: %s: %s", sc.Name, err)
					errs = append(errs, fmt.Errorf("StorageClass %s: %s", sc.Name, err))
				}
			case storagePolicyParameter:
				if err := checkStoragePolicy(ctx, v, infra); err != nil {
					klog.V(2).Infof("CheckStorageClasses: %s: %s", sc.Name, err)
					errs = append(errs, fmt.Errorf("StorageClass %s: %s", sc.Name, err))
				}
			}
		}
	}
	klog.V(4).Infof("CheckStorageClasses checked %d storage classes, %d problems found", len(scs), len(errs))
	return JoinErrors(errs)
}

// CheckPVs tests that datastore name in all PVs in the cluster is short enough.
func CheckPVs(ctx *CheckContext) error {
	var errs []error

	pvs, err := ctx.KubeClient.ListPVs(ctx.Context)
	if err != nil {
		return err
	}
	for i := range pvs {
		pv := &pvs[i]
		if pv.Spec.VsphereVolume == nil {
			continue
		}
		klog.V(4).Infof("Checking PV %s: %s", pv.Name, pv.Spec.VsphereVolume.VolumePath)
		err := checkVolumeName(pv.Spec.VsphereVolume.VolumePath)
		if err != nil {
			klog.V(2).Infof("CheckPVs: %s: %s", pv.Name, err)
			errs = append(errs, fmt.Errorf("PersistentVolume %s: %s", pv.Name, err))
		}
	}
	klog.V(2).Infof("CheckPVs: checked %d PVs, %d problems found", len(pvs), len(errs))
	return JoinErrors(errs)
}

// CheckDefaultDatastore checks that the default data store name in vSphere config file is short enough.
func CheckDefaultDatastore(ctx *CheckContext) error {
	infra, err := ctx.KubeClient.GetInfrastructure(ctx.Context)
	if err != nil {
		return err
	}

	dsName := ctx.VMConfig.Workspace.DefaultDatastore
	if err := checkDataStore(ctx, dsName, infra); err != nil {
		return fmt.Errorf("defaultDatastore %q in vSphere configuration: %s", dsName, err)
	}
	return nil
}

// checkStoragePolicy lists all compatible datastores and checks their names are short.
func checkStoragePolicy(ctx *CheckContext, policyName string, infrastructure *configv1.Infrastructure) error {
	klog.V(4).Infof("Checking storage policy %s", policyName)

	pbm, err := getPolicy(ctx, policyName)
	if err != nil {
		return err
	}
	if len(pbm) == 0 {
		return fmt.Errorf("error listing storage policy %s: policy not found", policyName)
	}
	if len(pbm) > 1 {
		return fmt.Errorf("error listing storage policy %s: multiple (%d) policies found", policyName, len(pbm))
	}

	dataStores, err := getPolicyDatastores(ctx, pbm[0].GetPbmProfile().ProfileId)
	if err != nil {
		return err
	}
	klog.V(4).Infof("Policy %q is compatible with datastores %v", policyName, dataStores)

	var errs []error
	for _, dataStore := range dataStores {
		err := checkDataStore(ctx, dataStore, infrastructure)
		if err != nil {
			errs = append(errs, fmt.Errorf("storage policy %s: %s", policyName, err))
		}
	}
	return JoinErrors(errs)
}

// checkStoragePolicy lists all datastores compatible with given policy.
func getPolicyDatastores(ctx *CheckContext, profileID types.PbmProfileId) ([]string, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	c, err := pbm.NewClient(tctx, ctx.VMClient)
	if err != nil {
		return nil, err
	}

	// Load all datastores in vSphere
	kind := []string{"Datastore"}
	m := view.NewManager(ctx.VMClient)

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	v, err := m.CreateContainerView(tctx, ctx.VMClient.ServiceContent.RootFolder, kind, true)
	if err != nil {
		return nil, err
	}

	var content []vim.ObjectContent
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err = v.Retrieve(tctx, kind, []string{"Name"}, &content)
	_ = v.Destroy(tctx)
	if err != nil {
		return nil, err
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
	res, err := c.CheckRequirements(tctx, hubs, nil, req)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	rtype := types.PbmProfileResourceType{
		ResourceType: string(types.PbmProfileResourceTypeEnumSTORAGE),
	}
	category := types.PbmProfileCategoryEnumREQUIREMENT

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	ids, err := c.QueryProfile(tctx, rtype, string(category))
	if err != nil {
		return nil, err
	}

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	profiles, err := c.RetrieveContent(tctx, ids)
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		if p.GetPbmProfile().Name == name {
			return []types.BasePbmProfile{p}, nil
		}
	}
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	return c.RetrieveContent(tctx, []types.PbmProfileId{{UniqueId: name}})
}

func checkDataStore(ctx *CheckContext, dsName string, infrastructure *configv1.Infrastructure) error {
	clusterID := infrastructure.Status.InfrastructureName
	volumeName := generateVolumeName(clusterID, "pvc-00000000-0000-0000-0000-000000000000", maxVolumeName)
	fullVolumeName := fmt.Sprintf("[%s] 00000000-0000-0000-0000-000000000000/%s.vmdk", dsName, volumeName)
	klog.V(4).Infof("Checking data store %q with potential volume Name %s", dsName, fullVolumeName)
	if err := checkVolumeName(fullVolumeName); err != nil {
		return fmt.Errorf("datastore %s: %s", dsName, err)
	}
	if err := checkForDatastoreCluster(ctx, dsName); err != nil {
		return err
	}
	return nil
}

func checkForDatastoreCluster(ctx *CheckContext, dataStoreName string) error {
	matchingDC, err := getDatacenter(ctx, ctx.VMConfig.Workspace.Datacenter)
	if err != nil {
		return err
	}
	// lets fetch the datastore
	finder := find.NewFinder(ctx.VMClient, false)
	finder.SetDatacenter(matchingDC)
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	ds, err := finder.Datastore(tctx, dataStoreName)
	if err != nil {
		klog.Errorf("error getting datastore %s: %v", dataStoreName, err)
		return nil
	}

	var dsMo mo.Datastore
	pc := property.DefaultCollector(matchingDC.Client())
	properties := []string{DatastoreInfoProperty, SummaryProperty}
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err = pc.RetrieveOne(tctx, ds.Reference(), properties, &dsMo)
	if err != nil {
		klog.Errorf("error getting properties of datastore %s: %v", dataStoreName, err)
		return nil
	}

	// list datastore cluster
	m := view.NewManager(ctx.VMClient)
	kind := []string{"StoragePod"}
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	v, err := m.CreateContainerView(tctx, ctx.VMClient.ServiceContent.RootFolder, kind, true)
	if err != nil {
		klog.Errorf("error listing datastore cluster: %+v", err)
		return nil
	}
	defer func() {
		v.Destroy(tctx)
	}()

	var content []mo.StoragePod
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err = v.Retrieve(tctx, kind, []string{SummaryProperty, "childEntity"}, &content)
	if err != nil {
		klog.Errorf("error retrieving datastore cluster properties: %+v", err)
		// it is possible that we do not actually have permission to fetch datastore clusters
		// in which case rather than throwing an error - we will silently return nil, so as
		// we don't trigger unnecessary alerts.
		return nil
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
				return fmt.Errorf("datastore %s is part of %s datastore cluster", tDS.Summary.Name, ds.Summary.Name)
			}
		}
	}
	klog.V(2).Infof("Checked datastore %s for SRDS - no problems found", dataStoreName)
	return nil
}

func getDatastore(ctx *CheckContext, ref vim.ManagedObjectReference) (mo.Datastore, error) {
	var dsMo mo.Datastore
	pc := property.DefaultCollector(ctx.VMClient)
	properties := []string{DatastoreInfoProperty, SummaryProperty}
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	err := pc.RetrieveOne(tctx, ref, properties, &dsMo)
	if err != nil {
		return dsMo, err
	}
	return dsMo, nil
}

func checkVolumeName(name string) error {
	path := fmt.Sprintf("/var/lib/kubelet/plugins/kubernetes.io/vsphere-volume/mounts/%s", name)
	escapedPath, err := systemdEscape(path)
	if err != nil {
		return fmt.Errorf("error running systemd-escape: %s", err)
	}
	if len(escapedPath) >= 255 {
		return fmt.Errorf("datastore name is too long: escaped volume path %q must be under 255 characters, got %d", escapedPath, len(escapedPath))
	}
	return nil
}

func systemdEscape(path string) (string, error) {
	cmd := exec.Command("systemd-escape", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("systemd-escape: %s: %s", err, string(out))
	}
	escapedPath := strings.TrimSpace(string(out))
	return escapedPath, nil
}

// Copied from https://github.com/kubernetes/kubernetes/blob/93d288e2a47fa6d497b50d37c8b3a04e91da4228/pkg/volume/util/util.go#L230
// GenerateVolumeName returns a PV name with clusterName prefix. The function
// should be used to generate a name of GCE PD or Cinder volume. It basically
// adds "<clusterName>-dynamic-" before the PV name, making sure the resulting
// string fits given length and cuts "dynamic" if not.
func generateVolumeName(clusterName, pvName string, maxLength int) string {
	prefix := clusterName + "-dynamic"
	pvLen := len(pvName)

	// cut the "<clusterName>-dynamic" to fit full pvName into maxLength
	// +1 for the '-' dash
	if pvLen+1+len(prefix) > maxLength {
		prefix = prefix[:maxLength-pvLen-1]
	}
	return prefix + "-" + pvName
}
