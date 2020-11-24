package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/legacy-cloud-providers/vsphere"
)

// CheckNodes tests that Nodes have spec.providerID (i.e. they run with a cloud provider)
// and all nodes have disk.enableUUID enabled.
func CheckNodes(ctx context.Context, vmConfig *vsphere.VSphereConfig, vmClient *vim25.Client, kubeClient KubeClient) error {
	klog.V(4).Infof("CheckNodes started")

	nodes, err := kubeClient.ListNodes(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for i := range nodes {
		node := &nodes[i]

		err := checkNode(ctx, node, vmClient, vmConfig)
		if err != nil {
			errs = append(errs, fmt.Errorf("error on node %q: %s", node.Name, err))
			klog.V(2).Infof("Error on node %q: %s", node.Name, err)
		}
	}

	if len(errs) != 0 {
		return errors.NewAggregate(errs)
	}
	klog.Infof("CheckNodes succeeded, %d nodes checked", len(nodes))
	return nil
}

func checkNode(ctx context.Context, node *v1.Node, vmClient *vim25.Client, config *vsphere.VSphereConfig) error {
	klog.V(4).Infof("Checking node %q", node.Name)
	if node.Spec.ProviderID == "" {
		return fmt.Errorf("the node has no providerID")
	}
	klog.V(4).Infof("... the node has providerID: %s", node.Spec.ProviderID)

	if !strings.HasPrefix(node.Spec.ProviderID, "vsphere://") {
		return fmt.Errorf("the node's providerID does not start with vsphere://")
	}

	if err := checkDiskUUID(ctx, node, vmClient, config); err != nil {
		return err
	}
	return nil
}

func checkDiskUUID(ctx context.Context, node *v1.Node, vmClient *vim25.Client, config *vsphere.VSphereConfig) error {
	vm, err := getVM(ctx, node, vmClient, config)
	if err != nil {
		return err
	}

	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()
	var o mo.VirtualMachine
	err = vm.Properties(tctx, vm.Reference(), []string{"config.extraConfig", "config.flags"}, &o)
	if err != nil {
		return fmt.Errorf("failed to load VM %s: %s", node.Name, err)
	}

	if o.Config.Flags.DiskUuidEnabled == nil {
		return fmt.Errorf("the node has empty disk.enableUUID")
	}
	if *o.Config.Flags.DiskUuidEnabled == false {
		return fmt.Errorf("the node has disk.enableUUID = FALSE")
	}
	klog.V(4).Infof("... the node has correct disk.enableUUID")

	return nil
}

func getVM(ctx context.Context, node *v1.Node, vmClient *vim25.Client, config *vsphere.VSphereConfig) (*object.VirtualMachine, error) {
	tctx, cancel := context.WithTimeout(ctx, *Timeout)
	defer cancel()

	finder := find.NewFinder(vmClient, false)
	dc, err := finder.Datacenter(tctx, config.Workspace.Datacenter)
	if err != nil {
		return nil, fmt.Errorf("failed to access Datacenter %s: %s", config.Workspace.Datacenter, err)
	}

	s := object.NewSearchIndex(dc.Client())
	vmUUID := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(node.Spec.ProviderID, "vsphere://")))
	tctx, cancel = context.WithTimeout(ctx, *Timeout)
	defer cancel()
	svm, err := s.FindByUuid(tctx, dc, vmUUID, true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM by UUID %s: %s", vmUUID, err)
	}
	if svm == nil {
		return nil, fmt.Errorf("unable to find VM by UUID %s", vmUUID)
	}
	return object.NewVirtualMachine(vmClient, svm.Reference()), nil
}
