package check

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift/vsphere-problem-detector/pkg/check/mock"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vim25types "github.com/vmware/govmomi/vim25/types"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func getAuthManagerWithValidPrivileges(ctx *CheckContext, mockCtrl *gomock.Controller) (AuthManager, error) {
	finder := find.NewFinder(ctx.VMClient)
	validPermissionsAuthManagerClient, err := buildAuthManagerClient(ctx.Context, mockCtrl, finder, defaultUsername, nil, []string{})
	if err != nil {
		return nil, err
	}
	return validPermissionsAuthManagerClient, nil
}

func buildPermissionGroup(authManagerMock *mock.MockAuthManager,
	managedObjectRef vim25types.ManagedObjectReference,
	username string,
	permissions []string,
	groupName permissionGroup,
	overrideGroup *permissionGroup) {
	permissionToApply := permissions
	if overrideGroup != nil && *overrideGroup == groupName {
		permissionToApply = permissionToApply[:len(permissionToApply)-1]
	}
	authManagerMock.EXPECT().FetchUserPrivilegeOnEntities(gomock.Any(), []vim25types.ManagedObjectReference{
		managedObjectRef,
	}, username).Return([]vim25types.UserPrivilegeResult{
		{
			Privileges: permissionToApply,
		},
	}, nil).AnyTimes()
}

func buildAuthManagerClient(ctx context.Context, mockCtrl *gomock.Controller, finder *find.Finder, username string, overrideGroup *permissionGroup, folders []string) (AuthManager, error) {
	authManagerClient := mock.NewMockAuthManager(mockCtrl)
	for groupName, group := range permissions {
		switch groupName {
		case permissionVcenter:
			vcenter, err := finder.Folder(ctx, "/")
			if err != nil {
				return nil, err
			}
			buildPermissionGroup(authManagerClient, vcenter.Reference(), username, group, groupName, overrideGroup)
		case permissionDatacenter:
			datacenters, err := finder.DatacenterList(ctx, "/...")
			if err != nil {
				return nil, err
			}
			for _, datacenter := range datacenters {
				buildPermissionGroup(authManagerClient, datacenter.Reference(), username, group, groupName, overrideGroup)
			}
		case permissionDatastore:
			datastores, err := finder.DatastoreList(ctx, "/...")
			if err != nil {
				return nil, err
			}
			for _, datastore := range datastores {
				buildPermissionGroup(authManagerClient, datastore.Reference(), username, group, groupName, overrideGroup)
			}
		case permissionCluster:
			clusters, err := finder.ClusterComputeResourceList(ctx, "/...")
			if err != nil {
				return nil, err
			}
			for _, cluster := range clusters {
				buildPermissionGroup(authManagerClient, cluster.Reference(), username, group, groupName, overrideGroup)
			}
		case permissionPortgroup:
			networks, err := finder.NetworkList(ctx, "/...")
			if err != nil {
				return nil, err
			}
			for _, network := range networks {
				buildPermissionGroup(authManagerClient, network.Reference(), username, group, groupName, overrideGroup)
			}
		case permissionFolder:
			for _, folder := range folders {
				folder, err := finder.Folder(ctx, folder)
				if err != nil {
					return nil, err
				}
				buildPermissionGroup(authManagerClient, folder.Reference(), username, group, groupName, overrideGroup)
			}
		}
	}
	return authManagerClient, nil
}

func clusterLevelPrivilegeCheck(ctx *CheckContext) error {
	return CheckAccountPermissions(ctx)
}

func vmLevelPrivilegeCheck(ctx *CheckContext) error {
	nodePrivileges := &CheckComputeClusterPermissions{}
	nodePrivileges.StartCheck()
	finder := find.NewFinder(ctx.VMClient)
	virtualMachines, err := finder.VirtualMachineList(ctx.Context, root)
	if err != nil {
		return err
	}
	if len(virtualMachines) == 0 {
		return errors.New("no virtual machines found")
	}
	hosts, err := finder.HostSystemList(ctx.Context, root)
	if len(hosts) == 0 {
		return errors.New("no hosts found")
	}

	var vmToCheck *object.VirtualMachine
	for _, vm := range virtualMachines {
		hs, _ := vm.HostSystem(ctx.Context)
		hostProperties := []string{"summary", "parent"}
		var hostMo mo.HostSystem
		err = vm.Properties(ctx.Context, hs.Reference(), hostProperties, &hostMo)
		if err != nil {
			return errors.New("error getting host mo reference")
		}
		if hostMo.Parent.Type == "ClusterComputeResource" {
			vmToCheck = vm
		}
	}
	if vmToCheck == nil {
		return errors.New("unable to find virtual machine that is a member of a cluster")
	}
	var vmMo mo.VirtualMachine
	err = vmToCheck.Properties(ctx.Context, vmToCheck.Reference(), NodeProperties, &vmMo)
	if err != nil {
		return errors.New("error getting vm mo reference")
	}
	return nodePrivileges.CheckNode(ctx, nil, &vmMo)
}

func TestPermissionValidate(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	kubeClient := &fakeKubeClient{
		infrastructure: infrastructure(),
		nodes:          defaultNodes(),
	}

	simctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	finder := find.NewFinder(simctx.VMClient)

	if err != nil {
		t.Error(err)
		return
	}

	var folders = []string{"/DC0/vm"}
	validPermissionsAuthManagerClient, err := buildAuthManagerClient(ctx, mockCtrl, finder, defaultUsername, nil, folders)
	if err != nil {
		t.Error(err)
		return
	}

	missingVCenterPermissionsClient, err := buildAuthManagerClient(ctx, mockCtrl, finder, defaultUsername, &permissionVcenter, folders)
	if err != nil {
		t.Error(err)
		return
	}

	missingClusterPermissionsClient, err := buildAuthManagerClient(ctx, mockCtrl, finder, defaultUsername, &permissionCluster, folders)
	if err != nil {
		t.Error(err)
		return
	}

	missingDatastorePermissionsClient, err := buildAuthManagerClient(ctx, mockCtrl, finder, defaultUsername, &permissionDatastore, folders)
	if err != nil {
		t.Error(err)
		return
	}

	missingDatacenterPermissionsClient, err := buildAuthManagerClient(ctx, mockCtrl, finder, defaultUsername, &permissionDatacenter, folders)
	if err != nil {
		t.Error(err)
		return
	}

	missingFolderPermissionsClient, err := buildAuthManagerClient(ctx, mockCtrl, finder, defaultUsername, &permissionFolder, folders)
	if err != nil {
		t.Error(err)
		return
	}

	tests := []struct {
		name             string
		expectErr        string
		validationMethod func(*CheckContext) error
		authManager      AuthManager
	}{
		{
			name:             "valid Permissions",
			authManager:      validPermissionsAuthManagerClient,
			validationMethod: clusterLevelPrivilegeCheck,
		},
		{
			name:             "missing vCenter Permissions",
			authManager:      missingVCenterPermissionsClient,
			expectErr:        "missing privileges for vcenter: StorageProfile.View",
			validationMethod: clusterLevelPrivilegeCheck,
		},
		{
			name:             "missing cluster Permissions",
			authManager:      missingClusterPermissionsClient,
			expectErr:        "missing privileges for compute cluster DC1_C0: VirtualMachine.Config.AddNewDisk",
			validationMethod: vmLevelPrivilegeCheck,
		},
		{
			name:             "missing datacenter Permissions",
			authManager:      missingDatacenterPermissionsClient,
			expectErr:        "missing privileges for datacenter DC0: System.Read",
			validationMethod: clusterLevelPrivilegeCheck,
		},
		{
			name:             "missing datastore Permissions",
			authManager:      missingDatastorePermissionsClient,
			expectErr:        "missing privileges for datastore LocalDS_0: InventoryService.Tagging.ObjectAttachable",
			validationMethod: clusterLevelPrivilegeCheck,
		},
		{
			name:             "missing user-defined folder Permissions",
			authManager:      missingFolderPermissionsClient,
			expectErr:        "missing privileges for folder: VirtualMachine.Provisioning.DeployTemplate",
			validationMethod: clusterLevelPrivilegeCheck,
		},
	}

	for _, test := range tests {
		simctx.AuthManager = test.authManager
		t.Run(test.name, func(t *testing.T) {
			err := test.validationMethod(simctx)
			if test.expectErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Regexp(t, test.expectErr, err)
			}
		})
	}
}
