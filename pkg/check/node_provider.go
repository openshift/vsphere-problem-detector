package check

import (
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CheckNodeProviderID makes sure that all nodes have ProviderID set.
type CheckNodeProviderID struct{}

var _ NodeCheck = &CheckNodeProviderID{}

func (c *CheckNodeProviderID) Name() string {
	return "CheckNodeProviderID"
}

func (c *CheckNodeProviderID) StartCheck() error {
	return nil
}

func (c *CheckNodeProviderID) CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	if node.Spec.ProviderID == "" {
		return fmt.Errorf("the node has no node.spec.providerID configured")
	}
	klog.V(4).Infof("... the node has providerID: %s", node.Spec.ProviderID)
	return nil
}

func (c *CheckNodeProviderID) FinishCheck(ctx *CheckContext) {
	return
}
