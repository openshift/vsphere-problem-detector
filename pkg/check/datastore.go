package check

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi/pbm"
	"github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	vim "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/vsphere"
)

const (
	dsParameter            = "datastore"
	storagePolicyParameter = "storagepolicyname"
)

// CheckDatastoreNames tests that datastore names used anywhere in the cluster have the right length.
func CheckDatastoreNames(ctx context.Context, vmConfig *vsphere.VSphereConfig, vmClient *vim25.Client, kubeClient KubeClient) error {
	var errs []error
	if scerrs := checkStorageClasses(ctx, vmClient, kubeClient); len(scerrs) != 0 {
		errs = append(errs, scerrs...)
	}
	if pverrs := checkPVs(ctx, kubeClient); len(pverrs) != 0 {
		errs = append(errs, pverrs...)
	}
	if err := checkDefaultDatastore(ctx, vmConfig, kubeClient); err != nil {
		errs = append(errs, err)
	}
	return errors.NewAggregate(errs)
}

// checkStorageClasses tests that datastore name in all StorageClasses in the cluster is short enough.
func checkStorageClasses(ctx context.Context, vmClient *vim25.Client, kubeClient KubeClient) []error {
	var errs []error
	klog.V(4).Infof("checkStorageClasses started")

	infra, err := kubeClient.GetInfrastructure(ctx)
	if err != nil {
		return []error{err}
	}

	scs, err := kubeClient.ListStorageClasses(ctx)
	if err != nil {
		return []error{err}
	}
	for i := range scs {
		sc := &scs[i]
		if sc.Provisioner != "kubernetes.io/vsphere-volume" {
			klog.V(4).Infof("Skipping storage class %q: not a vSphere class", sc.Name)
			continue
		}

		for k, v := range sc.Parameters {
			switch strings.ToLower(k) {
			case dsParameter:
				if err := checkDataStore(v, infra); err != nil {
					errs = append(errs, fmt.Errorf("StorageClass %q is invalid: %s", sc.Name, err))
				}
			case storagePolicyParameter:
				if err := checkStoragePolicy(ctx, v, infra, vmClient); err != nil {
					errs = append(errs, fmt.Errorf("StorageClass %q is invalid: %s", sc.Name, err))
				}
			}
		}
	}
	klog.V(4).Infof("checkStorageClasses checked %d storage classes, %d problems found", len(scs), len(errs))
	return errs
}

// checkPVs tests that datastore name in all PVs in the cluster is short enough.
func checkPVs(ctx context.Context, kubeClient KubeClient) []error {
	var errs []error
	klog.V(4).Infof("checkPVs started")

	pvs, err := kubeClient.ListPVs(ctx)
	if err != nil {
		return []error{err}
	}
	for i := range pvs {
		pv := &pvs[i]
		if pv.Spec.VsphereVolume == nil {
			continue
		}
		klog.V(4).Infof("Checking PV %q: %s", pv.Name, pv.Spec.VsphereVolume.VolumePath)
		err := checkVolumeName(pv.Spec.VsphereVolume.VolumePath)
		if err != nil {
			errs = append(errs, fmt.Errorf("PersistentVolume %q is invalid: %s", pv.Name, err))
		}
	}
	klog.V(4).Infof("checkPVs checked %d PVs, %d problems found", len(pvs), len(errs))
	return errs
}

// checkDefaultDatastore checks that the default data store name in vSphere config file is short enough.
func checkDefaultDatastore(ctx context.Context, vmConfig *vsphere.VSphereConfig, kubeClient KubeClient) error {
	klog.V(4).Infof("checkDefaultDatastore started")
	infra, err := kubeClient.GetInfrastructure(ctx)
	if err != nil {
		return err
	}

	dsName := vmConfig.Workspace.DefaultDatastore
	if err := checkDataStore(dsName, infra); err != nil {
		return fmt.Errorf("defaultDatastore %q in vSphere configuration is invalid: %s", dsName, err)
	}
	klog.V(4).Infof("checkDefaultDatastore succeeded")
	return nil
}

// checkStoragePolicy lists all compatible datastores and checks their names are short.
func checkStoragePolicy(ctx context.Context, policyName string, infrastructure *configv1.Infrastructure, vmClient *vim25.Client) error {
	klog.V(4).Infof("Checking storage policy %q", policyName)

	pbm, err := getPolicy(ctx, policyName, vmClient)
	if err != nil {
		return err
	}
	if len(pbm) == 0 {
		return fmt.Errorf("error listing storage policy %q: policy not found", policyName)
	}
	if len(pbm) > 1 {
		return fmt.Errorf("error listing storage policy %q: multiple (%d) policies found", policyName, len(pbm))
	}

	dataStores, err := getPolicyDatastores(ctx, pbm[0].GetPbmProfile().ProfileId, vmClient)
	if err != nil {
		return err
	}
	klog.V(4).Infof("Policy %q is compatible with datastores %v", policyName, dataStores)

	var errs []error
	for _, dataStore := range dataStores {
		err := checkDataStore(dataStore, infrastructure)
		if err != nil {
			errs = append(errs, fmt.Errorf("storage policy %q: %s", policyName, err))
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// checkStoragePolicy lists all datastores compatible with given policy.
func getPolicyDatastores(ctx context.Context, profileID types.PbmProfileId, vmClient *vim25.Client) ([]string, error) {
	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()
	c, err := pbm.NewClient(tctx, vmClient)
	if err != nil {
		return nil, err
	}

	// Load all datastores in vSphere
	kind := []string{"Datastore"}
	m := view.NewManager(vmClient)

	tctx, cancel = context.WithTimeout(ctx, *Timeout)
	defer cancel()
	v, err := m.CreateContainerView(tctx, vmClient.ServiceContent.RootFolder, kind, true)
	if err != nil {
		return nil, err
	}

	var content []vim.ObjectContent
	tctx, cancel = context.WithTimeout(ctx, *Timeout)
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

	tctx, cancel = context.WithTimeout(ctx, *Timeout)
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

func getPolicy(pctx context.Context, name string, vmClient *vim25.Client) ([]types.BasePbmProfile, error) {
	ctx, cancel := context.WithTimeout(pctx, *Timeout)
	defer cancel()

	c, err := pbm.NewClient(ctx, vmClient)
	if err != nil {
		return nil, err
	}
	rtype := types.PbmProfileResourceType{
		ResourceType: string(types.PbmProfileResourceTypeEnumSTORAGE),
	}
	category := types.PbmProfileCategoryEnumREQUIREMENT

	ctx, cancel = context.WithTimeout(pctx, *Timeout)
	defer cancel()
	ids, err := c.QueryProfile(ctx, rtype, string(category))
	if err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(pctx, *Timeout)
	defer cancel()
	profiles, err := c.RetrieveContent(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		if p.GetPbmProfile().Name == name {
			return []types.BasePbmProfile{p}, nil
		}
	}
	ctx, cancel = context.WithTimeout(pctx, *Timeout)
	defer cancel()
	return c.RetrieveContent(ctx, []types.PbmProfileId{{UniqueId: name}})
}

func checkDataStore(dsName string, infrastructure *configv1.Infrastructure) error {
	klog.V(4).Infof("Checking datastore %q", dsName)
	clusterID := infrastructure.Status.InfrastructureName
	volumeName := fmt.Sprintf("[%s] 00000000-0000-0000-0000-000000000000/%s-dynamic-pvc-00000000-0000-0000-0000-000000000000.vmdk", dsName, clusterID)
	klog.V(4).Infof("Checking data store %q with potential volume Name %s", dsName, volumeName)
	if err := checkVolumeName(volumeName); err != nil {
		return fmt.Errorf("error checking datastore %q: %s", dsName, err)
	}

	return nil
}

func checkVolumeName(name string) error {
	path := fmt.Sprintf("/var/lib/kubelet/plugins/kubernetes.io/vsphere-volume/mounts/%s", name)
	escapedPath, err := systemdEscape(path)
	if err != nil {
		return fmt.Errorf("error running systemd-escape: %s", err)
	}
	if len(path) >= 255 {
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
	klog.V(4).Infof("path %q systemd-escaped to %q (%d)", path, escapedPath, len(escapedPath))
	return escapedPath, nil
}
