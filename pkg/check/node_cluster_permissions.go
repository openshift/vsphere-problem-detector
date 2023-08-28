package check

import (
	"errors"
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CheckComputeClusterPermissions confirms that resources associated with the node maintain required privileges.
type CheckComputeClusterPermissions struct {
	computeClusters map[string]*mo.ClusterComputeResource
}

var _ NodeCheck = &CheckComputeClusterPermissions{}

func (c *CheckComputeClusterPermissions) Name() string {
	return "CheckComputeClusterPermissions"
}

func (c *CheckComputeClusterPermissions) StartCheck() error {
	c.computeClusters = make(map[string]*mo.ClusterComputeResource)
	return nil
}

func (c *CheckComputeClusterPermissions) checkComputeClusterPrivileges(ctx *CheckContext, vm *mo.VirtualMachine, readOnly bool) *CheckError {
	cluster, err := getComputeCluster(ctx, vm.Runtime.Host.Reference())
	if err != nil {
		klog.Infof("compute cluster resource could not be obtained for %v", vm.Reference())
		return nil
	}

	if readOnly {
		// Having read only privilege is implied if we don't trigger the error above.
		return nil
	}

	if _, ok := c.computeClusters[cluster.Name]; ok {
		klog.Infof("privileges for compute cluster %v have already been checked", cluster.Name)
		return nil
	}
	c.computeClusters[cluster.Name] = cluster

	if _, ok := ctx.VMConfig.VirtualCenter[ctx.VMConfig.Workspace.VCenterIP]; !ok {
		return &CheckError{"vcenter_not_found", errors.New("vcenter instance not found in the virtual center map")}
	}

	if err := comparePrivileges(ctx.Context, ctx.Username, cluster.Reference(), ctx.AuthManager, permissions[permissionCluster]); err != nil {
		return &CheckError{"node_missing_permissions", fmt.Errorf("missing privileges for compute cluster %s: %s", cluster.Name, err.Error())}
	}
	return nil
}

func (c *CheckComputeClusterPermissions) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) *CheckError {
	readOnly := false

	// If pre-existing resource pool was defined, only check cluster for read privilege
	if ctx.VMConfig.Workspace.ResourcePoolPath != "" {
		readOnly = true
	}

	checkError := c.checkComputeClusterPrivileges(ctx, vm, readOnly)
	if checkError != nil {
		return checkError
	}
	return nil
}

func (c *CheckComputeClusterPermissions) FinishCheck(ctx *CheckContext) {
	return
}
