package check

import (
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CheckResourcePoolPermissions confirms that resources associated with the node maintain required privileges.
type CheckResourcePoolPermissions struct {
	resourcePools map[string]*mo.ResourcePool
}

var _ NodeCheck = &CheckResourcePoolPermissions{}

func (c *CheckResourcePoolPermissions) Name() string {
	return "CheckResourcePoolPermissions"
}

func (c *CheckResourcePoolPermissions) StartCheck() error {
	c.resourcePools = make(map[string]*mo.ResourcePool)
	return nil
}

func (c *CheckResourcePoolPermissions) checkResourcePoolPrivileges(ctx *CheckContext, vm *mo.VirtualMachine) error {
	resourcePool, resourcePoolPath, err := getResourcePool(ctx, vm.Reference())
	if err != nil {
		klog.Infof("resource pool could not be obtained for %v", vm.Reference())
		return nil
	}

	if _, ok := c.resourcePools[resourcePoolPath]; ok {
		klog.Infof("privileges for resource pool %s have already been checked", resourcePoolPath)
	}
	c.resourcePools[resourcePoolPath] = resourcePool

	if err := comparePrivileges(ctx.Context, ctx.Username, resourcePool.Reference(), ctx.AuthManager, permissions[permissionCluster]); err != nil {
		return fmt.Errorf("missing privileges for resource pool %s: %s", resourcePoolPath, err.Error())
	}

	return nil
}

func (c *CheckResourcePoolPermissions) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	var errs []error

	if vm.ResourcePool != nil {
		rpObj := object.NewResourcePool(ctx.VMClient, *vm.ResourcePool)

		if rpObj != nil {
			rpName, err := rpObj.ObjectName(ctx.Context)

			if err != nil {
				return err
			}

			if strings.HasSuffix(rpName, "Resources") {
				return nil
			}

			err = c.checkResourcePoolPrivileges(ctx, vm)
			if err != nil {
				errs = append(errs, err)
			}
			if len(errs) > 0 {
				return join(errs)
			}
		} else {
			klog.Info("resource pool object is nil, skipping check")
		}
	} else {
		klog.Info("virtual machine object is nil, skipping check")
	}

	return nil
}

func (c *CheckResourcePoolPermissions) FinishCheck(ctx *CheckContext) {
	return
}
