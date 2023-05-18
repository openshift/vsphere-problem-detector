package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	"k8s.io/klog/v2"
)

const CbtProperty = "ctkEnabled"

type CBTResults struct {
	disabled []string
	enabled  []string
}

// CheckCBT this method will check all VMs for a cluster to make sure they have
// CBT configured the same across them.
func CheckCBT(ctx *CheckContext) error {
	klog.Info("Checking VMs for CBT settings.")
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	results := &CBTResults{}

	// Get nodes
	nodes, err := ctx.KubeClient.ListNodes(tctx)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		klog.Infof("Checking node %v", node.Name)

		// Get VM
		vm, err := getVirtualMachine(ctx, nil, node.Spec.ProviderID)
		if err != nil {
			return err
		}

		// Check for CBT property
		checkProperty(node.Name, vm, results)
	}

	// Compare results.  Either all VMs need to be enabled or disabled.  No mix and match.
	klog.V(2).Infof("Enabled (%v) Disabled (%v)", len(results.enabled), len(results.disabled))
	if len(results.enabled) > 0 && len(results.disabled) > 0 {
		klog.V(2).Infof("Detected mismatch environment.")
		return fmt.Errorf("CBT Feature: All nodes are not matching in configuration.  The following have CBT disabled: %v", results.disabled)
	}

	return nil
}

// checkProperty Looks through the VM extra configs to find the CBT property.  This will put
// the nodeName into the CBTResults object based on if the node is enabled or disabled.
func checkProperty(nodeName string, vm *mo.VirtualMachine, results *CBTResults) {
	propFound := false
	for _, config := range vm.Config.ExtraConfig {
		if config.GetOptionValue().Key == CbtProperty {
			klog.V(4).Infof("Found property with value %v", config.GetOptionValue().Value)
			if strings.EqualFold(fmt.Sprintf("%v", config.GetOptionValue().Value), "TRUE") {
				results.enabled = append(results.enabled, nodeName)
			} else {
				results.disabled = append(results.disabled, nodeName)
			}
			propFound = true
			break
		}
	}
	if !propFound {
		klog.V(4).Infof("Property no found for vm %v", nodeName)
		results.disabled = append(results.disabled, nodeName)
	}
}
