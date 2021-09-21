package check

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
)

// CheckFolderPermissions tests that OCP has permissions to list volumes in
// the default Datastore.
// This is necessary to create volumes.
// The check lists datastore's "/", which must exist.
// The check tries to list "kubevols/". It tolerates when it's missing,
// it will be created by OCP on the first provisioning.
func CheckFolderPermissions(ctx *CheckContext) error {
	dc, err := getDatacenter(ctx, ctx.VMConfig.Workspace.Datacenter)
	if err != nil {
		return err
	}

	ds, err := getDataStoreByName(ctx, ctx.VMConfig.Workspace.DefaultDatastore, dc)
	if err != nil {
		return fmt.Errorf("failed to access datastore %s: %s", ctx.VMConfig.Workspace.DefaultDatastore, err)
	}

	// OCP needs permissions to list files, try "/" that must exists.
	err = listDirectory(ctx, ds, "/", false)
	if err != nil {
		return err
	}

	// OCP needs permissions to list "/kubelet", tolerate if it does not exist.
	err = listDirectory(ctx, ds, "/kubevols", true)
	if err != nil {
		return err
	}
	return nil
}

func listDirectory(ctx *CheckContext, ds *object.Datastore, path string, tolerateNotFound bool) error {
	klog.V(4).Infof("Listing datastore %s path %s", ds.Name(), path)
	dsName := ds.Name()

	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	browser, err := ds.Browser(tctx)
	if err != nil {
		return fmt.Errorf("failed to create browser for datastore %s: %s", dsName, err)
	}

	spec := types.HostDatastoreBrowserSearchSpec{
		MatchPattern: []string{"*"},
	}
	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	task, err := browser.SearchDatastore(tctx, ds.Path(path), &spec)
	if err != nil {
		if tolerateNotFound && types.IsFileNotFound(err) {
			klog.V(2).Infof(
				"Warning: path %s does not exist it datastore %s. It will be created by kubernetes-controller-manager on the first dynamic provisioning.",
				path,
				dsName)
			return nil
		}
		// we may not have sufficient permissions to browse datastore path
		klog.V(2).Infof("failed to browse datastore %s at path %s: %v", dsName, path, err)
		return nil
	}

	tctx, cancel = context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()
	info, err := task.WaitForResult(tctx, nil)
	if err != nil {
		if tolerateNotFound && types.IsFileNotFound(err) {
			klog.V(2).Infof(
				"Warning: path %s does not exist it datastore %s. It will be created by kubernetes-controller-manager on the first dynamic provisioning.",
				path,
				dsName)
			return nil
		}
		return fmt.Errorf("failed to list datastore %s at path %s: %s ", dsName, path, err)
	}

	var items []types.HostDatastoreBrowserSearchResults
	switch r := info.Result.(type) {
	case types.HostDatastoreBrowserSearchResults:
		items = []types.HostDatastoreBrowserSearchResults{r}
	case types.ArrayOfHostDatastoreBrowserSearchResults:
		items = r.HostDatastoreBrowserSearchResults
	default:
		return fmt.Errorf("uknown data received from datastore %s browser: %T", dsName, r)
	}

	// In theory, there should be just one item, but list all to be sure
	for _, i := range items {
		klog.V(2).Infof("CheckFolderPermissions: found %d files in datastore %s at path %s", len(i.File), dsName, path)
	}
	return nil
}
