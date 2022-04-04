package check

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog/v2"
)

const root = "/..."

// formatPath is a helper function that appends "/..." to enable recursive
// find in a root object. For details, see the introduction at:
// https://godoc.org/github.com/vmware/govmomi/find
func formatPath(rootObject string) string {
	return fmt.Sprintf("%s/...", rootObject)
}

//go:generate mockgen -source=./permissions.go -destination=./mock/authmanager_generated.go -package=mock

// AuthManager defines an interface to an implementation of the AuthorizationManager to facilitate mocking
type AuthManager interface {
	FetchUserPrivilegeOnEntities(ctx context.Context, entities []types.ManagedObjectReference, userName string) ([]types.UserPrivilegeResult, error)
}

// permissionGroup is the group of permissions needed by cluster creation, operation, or teardown.
type permissionGroup string

// permissionGroupDefinition defines a group of permissions and a related human friendly description
type permissionGroupDefinition struct {
	/* Permissions array of privileges which correlate with the privileges listed in docs */
	Permissions []string
	/* Description friendly description of privilege group */
	Description string
}

var (
	permissionVcenter    permissionGroup = "vcenter"
	permissionCluster    permissionGroup = "cluster"
	permissionPortgroup  permissionGroup = "portgroup"
	permissionDatacenter permissionGroup = "datacenter"
	permissionDatastore  permissionGroup = "datastore"
	permissionFolder     permissionGroup = "folder"
)
var permissions = map[permissionGroup][]string{
	// Base set of permissions required for cluster creation
	permissionVcenter: {
		"Cns.Searchable",
		"InventoryService.Tagging.AttachTag",
		"InventoryService.Tagging.CreateCategory",
		"InventoryService.Tagging.CreateTag",
		"InventoryService.Tagging.DeleteCategory",
		"InventoryService.Tagging.DeleteTag",
		"InventoryService.Tagging.EditCategory",
		"InventoryService.Tagging.EditTag",
		"Sessions.ValidateSession",
		"StorageProfile.Update",
		"StorageProfile.View",
	},
	permissionCluster: {
		"Resource.AssignVMToPool",
		"VApp.AssignResourcePool",
		"VApp.Import",
		"VirtualMachine.Config.AddNewDisk",
	},
	permissionPortgroup: {
		"Network.Assign",
	},
	permissionFolder: {
		"Resource.AssignVMToPool",
		"VApp.Import",
		"VirtualMachine.Config.AddExistingDisk",
		"VirtualMachine.Config.AddNewDisk",
		"VirtualMachine.Config.AddRemoveDevice",
		"VirtualMachine.Config.AdvancedConfig",
		"VirtualMachine.Config.Annotation",
		"VirtualMachine.Config.CPUCount",
		"VirtualMachine.Config.DiskExtend",
		"VirtualMachine.Config.DiskLease",
		"VirtualMachine.Config.EditDevice",
		"VirtualMachine.Config.Memory",
		"VirtualMachine.Config.RemoveDisk",
		"VirtualMachine.Config.Rename",
		"VirtualMachine.Config.ResetGuestInfo",
		"VirtualMachine.Config.Resource",
		"VirtualMachine.Config.Settings",
		"VirtualMachine.Config.UpgradeVirtualHardware",
		"VirtualMachine.Interact.GuestControl",
		"VirtualMachine.Interact.PowerOff",
		"VirtualMachine.Interact.PowerOn",
		"VirtualMachine.Interact.Reset",
		"VirtualMachine.Inventory.Create",
		"VirtualMachine.Inventory.CreateFromExisting",
		"VirtualMachine.Inventory.Delete",
		"VirtualMachine.Provisioning.Clone",
		"VirtualMachine.Provisioning.MarkAsTemplate",
		"VirtualMachine.Provisioning.DeployTemplate",
	},
	permissionDatacenter: {
		"Resource.AssignVMToPool",
		"VApp.Import",
		"VirtualMachine.Config.AddExistingDisk",
		"VirtualMachine.Config.AddNewDisk",
		"VirtualMachine.Config.AddRemoveDevice",
		"VirtualMachine.Config.AdvancedConfig",
		"VirtualMachine.Config.Annotation",
		"VirtualMachine.Config.CPUCount",
		"VirtualMachine.Config.DiskExtend",
		"VirtualMachine.Config.DiskLease",
		"VirtualMachine.Config.EditDevice",
		"VirtualMachine.Config.Memory",
		"VirtualMachine.Config.RemoveDisk",
		"VirtualMachine.Config.Rename",
		"VirtualMachine.Config.ResetGuestInfo",
		"VirtualMachine.Config.Resource",
		"VirtualMachine.Config.Settings",
		"VirtualMachine.Config.UpgradeVirtualHardware",
		"VirtualMachine.Interact.GuestControl",
		"VirtualMachine.Interact.PowerOff",
		"VirtualMachine.Interact.PowerOn",
		"VirtualMachine.Interact.Reset",
		"VirtualMachine.Inventory.Create",
		"VirtualMachine.Inventory.CreateFromExisting",
		"VirtualMachine.Inventory.Delete",
		"VirtualMachine.Provisioning.Clone",
		"VirtualMachine.Provisioning.DeployTemplate",
		"VirtualMachine.Provisioning.MarkAsTemplate",
		"Folder.Create",
		"Folder.Delete",
	},
	permissionDatastore: {
		"Datastore.AllocateSpace",
		"Datastore.Browse",
		"Datastore.FileManagement",
		"InventoryService.Tagging.ObjectAttachable",
	},
}

func comparePrivileges(ctx context.Context, username string, mo types.ManagedObjectReference, authManager AuthManager, required []string) error {
	derived, err := authManager.FetchUserPrivilegeOnEntities(ctx, []types.ManagedObjectReference{mo}, username)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve privileges")
	}
	var missingPrivileges = ""
	for _, neededPrivilege := range required {
		var hasPrivilege = false
		for _, userPrivilege := range derived {
			for _, assignedPrivilege := range userPrivilege.Privileges {
				if assignedPrivilege == neededPrivilege {
					hasPrivilege = true
					break
				}
			}
		}
		if hasPrivilege == false {
			if missingPrivileges != "" {
				missingPrivileges = missingPrivileges + ", "
			}
			missingPrivileges = missingPrivileges + neededPrivilege
		}
	}
	if missingPrivileges != "" {
		return errors.Errorf(missingPrivileges)
	}
	return nil
}

func checkDatacenterPrivileges(ctx *CheckContext, dataCenterName string) error {
	if _, ok := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP]; !ok {
		return errors.New("vcenter instance not found in the virtual center map")
	}
	username := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP].User

	matchingDC, err := getDatacenter(ctx, dataCenterName)
	if err != nil {
		klog.Errorf("error getting datacenter %s: %v", ctx.VMConfig.Workspace.Datacenter, err)
		return err
	}
	if err := comparePrivileges(ctx.Context, username, matchingDC.Reference(), ctx.AuthManager, permissions[permissionDatacenter]); err != nil {
		return fmt.Errorf("missing privileges for datacenter %s: %s", dataCenterName, err.Error())
	}
	return nil
}

func checkFolderPrivileges(ctx *CheckContext, folderPath string, group permissionGroup) error {
	if _, ok := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP]; !ok {
		return errors.New("vcenter instance not found in the virtual center map")
	}
	username := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP].User

	finder := find.NewFinder(ctx.VMClient)
	folder, err := getFolderReference(ctx.Context, folderPath, finder)
	if err != nil {
		klog.Errorf("error getting folder %s: %v", folderPath, err)
		return err
	}
	if err := comparePrivileges(ctx.Context, username, folder.Reference(), ctx.AuthManager, permissions[group]); err != nil {
		return fmt.Errorf("missing privileges for %s: %s", group, err.Error())
	}
	return nil
}

func checkDatastorePrivileges(ctx *CheckContext, dataStoreName string) error {
	if _, ok := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP]; !ok {
		return errors.New("vcenter instance not found in the virtual center map")
	}

	username := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP].User

	dc, err := getDatacenter(ctx, ctx.VMConfig.Workspace.Datacenter)
	if err != nil {
		klog.Errorf("error getting datacenter %s: %v", ctx.VMConfig.Workspace.Datacenter, err)
		return err
	}
	ds, err := getDataStoreByName(ctx, dataStoreName, dc)
	if err != nil {
		klog.Errorf("error getting datastore %s: %v", dataStoreName, err)
		return err
	}
	if err := comparePrivileges(ctx.Context, username, ds.Reference(), ctx.AuthManager, permissions[permissionDatastore]); err != nil {
		return fmt.Errorf("missing privileges for datastore %s: %s", dataStoreName, err.Error())
	}
	return nil
}

func getFolderReference(ctx context.Context, path string, finder *find.Finder) (*types.ManagedObjectReference, error) {
	folderObj, err := finder.Folder(ctx, path)
	if err != nil {
		return nil, errors.Wrapf(err, "specified folder not found")
	}
	ref := folderObj.Reference()
	return &ref, nil
}

// CheckAccountPermissions will attempt to validate that the necessary credentials are held by the account performing the
// installation. each group of privileges will be checked for missing privileges.
func CheckAccountPermissions(ctx *CheckContext) error {
	var errs []error
	err := checkDatastorePrivileges(ctx, ctx.VMConfig.Workspace.DefaultDatastore)
	if err != nil {
		errs = append(errs, err)
	}

	err = checkDatacenterPrivileges(ctx, ctx.VMConfig.Workspace.Datacenter)
	if err != nil {
		errs = append(errs, err)
	}

	err = checkFolderPrivileges(ctx, "/", permissionVcenter)
	if err != nil {
		errs = append(errs, err)
	}

	if ctx.VMConfig.Workspace.Folder != "" {
		err = checkFolderPrivileges(ctx, ctx.VMConfig.Workspace.Folder, permissionFolder)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return join(errs)
	}
	return nil
}
