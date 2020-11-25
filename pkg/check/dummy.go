package check

import (
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
)

// CheckClusterDummy is a dummy check that always fails to test the operator & its exp. backoff.
func CheckClusterDummy(ctx *CheckContext) error {
	return fmt.Errorf("Dummy error")
}

// CheckNodeDummy is a dummy check that always fails to test the operator & its exp. backoff.
func CheckNodeDummy(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	return fmt.Errorf("Dummy error")
}
