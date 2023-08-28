package check

import (
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CheckNodeDiskUUID makes sure that all nodes have disk.enableUUID=TRUE.
type CheckNodeDiskUUID struct{}

var _ NodeCheck = &CheckNodeDiskUUID{}

func (c *CheckNodeDiskUUID) Name() string {
	return "CheckNodeDiskUUID"
}

func (c *CheckNodeDiskUUID) StartCheck() error {
	return nil
}

func (c *CheckNodeDiskUUID) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) *CheckError {
	if vm.Config.Flags.DiskUuidEnabled == nil {
		return &CheckError{"empty_disk_uuid", fmt.Errorf("the node has empty disk.enableUUID")}
	}
	if *vm.Config.Flags.DiskUuidEnabled == false {
		return &CheckError{"false_disk_uuid", fmt.Errorf("the node has disk.enableUUID = FALSE")}
	}
	klog.V(4).Infof("... the node has correct disk.enableUUID")
	return nil
}

func (c *CheckNodeDiskUUID) FinishCheck(ctx *CheckContext) {
	return
}
