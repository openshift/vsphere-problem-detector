package check

import (
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

func (c *CheckComputeClusterPermissions) checkComputeClusterPrivileges(ctx *CheckContext, vCenter *VCenter, vm *mo.VirtualMachine, readOnly bool) error {
	cluster, err := getComputeCluster(ctx, vCenter, vm.Runtime.Host.Reference())
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

	if err := comparePrivileges(ctx.Context, vCenter.Username, cluster.Reference(), vCenter.AuthManager, permissions[permissionCluster]); err != nil {
		return fmt.Errorf("missing privileges for compute cluster %s: %s", cluster.Name, err.Error())
	}
	return nil
}

func (c *CheckComputeClusterPermissions) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	var errs []error
	readOnly := false

	vCenterInfo, err := GetVCenter(ctx, node)
	if err != nil {
		return fmt.Errorf("unable to check node %s: %s", node.Name, err)
	}

	// If pre-existing resource pool was defined, only check cluster for read privilege
	if ctx.VMConfig.LegacyConfig != nil && ctx.VMConfig.LegacyConfig.Workspace.ResourcePoolPath != "" {
		readOnly = true
	}

	err = c.checkComputeClusterPrivileges(ctx, vCenterInfo, vm, readOnly)
	if err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return join(errs)
	}
	return nil
}

func (c *CheckComputeClusterPermissions) FinishCheck(ctx *CheckContext) {}
