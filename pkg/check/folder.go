package check

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/vsphere"
)

// CheckFolderList tests that OCP has permissions to list volumes in Datastore.
// This is necessary to create volumes.
// The check lists datastore's "/", which must exist.
// The check tries to list "kubevols/". It tolerates when it's missing,
// it will be created by OCP on the first provisioning.
func CheckFolderList(ctx context.Context, vmConfig *vsphere.VSphereConfig, vmClient *vim25.Client, kubeClient KubeClient) error {
	klog.V(4).Infof("CheckFolderList started")

	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()

	finder := find.NewFinder(vmClient, false)
	dc, err := finder.Datacenter(tctx, vmConfig.Workspace.Datacenter)
	if err != nil {
		return fmt.Errorf("failed to access datacenter %s: %s", vmConfig.Workspace.Datacenter, err)
	}

	tctx, cancel = context.WithTimeout(ctx, *Timeout)
	defer cancel()
	finder.SetDatacenter(dc)
	ds, err := finder.Datastore(tctx, vmConfig.Workspace.DefaultDatastore)
	if err != nil {
		return fmt.Errorf("failed to access datastore %s: %s", vmConfig.Workspace.DefaultDatastore, err)
	}
	// OCP needs permissions to list files, try "/" that must exists.
	err = listDirectory(ctx, vmConfig, ds, "/", false)
	if err != nil {
		return err
	}

	// OCP needs permissions to list "/kubelet", tolerate if it does not exist.
	err = listDirectory(ctx, vmConfig, ds, "/kubevols", true)
	if err != nil {
		return err
	}

	klog.Infof("Listing Datastore %q succeeded", vmConfig.Workspace.DefaultDatastore)
	return nil
}

func listDirectory(ctx context.Context, config *vsphere.VSphereConfig, ds *object.Datastore, path string, tolerateNotFound bool) error {
	klog.V(4).Infof("Listing datastore %s path %s", ds.Name(), path)
	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()

	browser, err := ds.Browser(tctx)
	if err != nil {
		return fmt.Errorf("failed to create browser for datastore %s: %s", config.Workspace.DefaultDatastore, err)
	}

	spec := types.HostDatastoreBrowserSearchSpec{
		MatchPattern: []string{"*"},
	}
	tctx, cancel = context.WithTimeout(ctx, *Timeout)
	defer cancel()
	task, err := browser.SearchDatastore(tctx, ds.Path(path), &spec)
	if err != nil {
		if tolerateNotFound && types.IsFileNotFound(err) {
			klog.Infof("Warning: path %s does not exist it datastore %s", path, config.Workspace.DefaultDatastore)
			return nil
		}
		return fmt.Errorf("failed to browse datastore %s: %s", config.Workspace.DefaultDatastore, err)
	}

	tctx, cancel = context.WithTimeout(ctx, *Timeout)
	defer cancel()
	info, err := task.WaitForResult(tctx, nil)
	if err != nil {
		if tolerateNotFound && types.IsFileNotFound(err) {
			klog.Infof("Warning: path %s does not exist it datastore %s", path, config.Workspace.DefaultDatastore)
			return nil
		}
		return fmt.Errorf("failed to list datastore: %s ", err)
	}

	var items []types.HostDatastoreBrowserSearchResults
	switch r := info.Result.(type) {
	case types.HostDatastoreBrowserSearchResults:
		items = []types.HostDatastoreBrowserSearchResults{r}
	case types.ArrayOfHostDatastoreBrowserSearchResults:
		items = r.HostDatastoreBrowserSearchResults
	default:
		return fmt.Errorf("uknown data received from datastore browser: %T", r)
	}

	for _, i := range items {
		for _, f := range i.File {
			klog.V(4).Infof("Found file %s/%s", path, f.GetFileInfo().Path)
		}
	}
	return nil

}
