package check

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/session"
	k8sv1 "k8s.io/api/core/v1"

	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

// helper to build a missing-cluster-permissions AuthManager and attach it
func setMissingClusterPermissionsAuthManager(t *testing.T, ctx *CheckContext) {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)

	// Use the first vCenter (dc0 in simulator configs)
	vcenter := ctx.VCenters["dc0"]
	finder := find.NewFinder(vcenter.VMClient)
	sessionMgr := session.NewManager(vcenter.VMClient)
	userSession, err := sessionMgr.UserSession(ctx.Context)
	if err != nil {
		t.Fatalf("failed to get user session: %v", err)
	}

	authMgr, err := buildAuthManagerClient(context.TODO(), mockCtrl, finder, userSession.UserName, &permissionCluster, []string{})
	if err != nil {
		t.Fatalf("failed to build mock auth manager: %v", err)
	}
	ctx.VCenters["dc0"].AuthManager = authMgr
}

func TestCheckComputeClusterPermissions_LegacyINI_ReadOnly(t *testing.T) {
	// Stage
	check := &CheckComputeClusterPermissions{}
	if err := check.StartCheck(); err != nil {
		t.Fatalf("StartCheck failed: %v", err)
	}

	node := testlib.Node("DC0_H0_VM0")
	kubeClient := &testlib.FakeKubeClient{
		Infrastructure: testlib.Infrastructure(), // legacy infra shape is fine
		Nodes:          []*k8sv1.Node{node},
	}
	ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Force legacy path: non-empty ResourcePoolPath implies read-only cluster check
	if ctx.VMConfig.LegacyConfig == nil {
		t.Fatalf("expected legacy config to be present in simulator setup")
	}
	ctx.VMConfig.LegacyConfig.Workspace.ResourcePoolPath = "/DC0/host/DC0_C0/Resources/custom"

	// Ensure cluster permissions would be missing if checked (to prove read_only skips)
	setMissingClusterPermissionsAuthManager(t, ctx)

	// Get VM for node
	vCenter, err := GetVCenter(ctx, node)
	if err != nil {
		t.Fatalf("error getting vCenter for node %s: %s", node.Name, err)
	}
	vm, err := testlib.GetVM(vCenter.VMClient, node)
	if err != nil {
		t.Fatalf("error getting vm for node %s: %s", node.Name, err)
	}

	// Act
	err = check.CheckNode(ctx, node, vm)

	// Assert: should NOT error due to read-only path
	assert.NoError(t, err)
}

func TestCheckComputeClusterPermissions_Infrastructure_ReadOnly_WithCustomRP(t *testing.T) {
	// Stage
	check := &CheckComputeClusterPermissions{}
	if err := check.StartCheck(); err != nil {
		t.Fatalf("StartCheck failed: %v", err)
	}

	// Node with region/zone labels matching the failure domain
	node := testlib.Node("DC0_H0_VM0", func(n *k8sv1.Node) {
		if n.Labels == nil {
			n.Labels = map[string]string{}
		}
		n.Labels["topology.kubernetes.io/region"] = "east"
		n.Labels["topology.kubernetes.io/zone"] = "east-1a"
	})

	infra := testlib.InfrastructureWithFailureDomain(func(inf *ocpv1.Infrastructure) {
		// Set a custom ResourcePool path (not ending with /Resources) to trigger read_only
		inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.ResourcePool = "/DC0/host/DC0_C0/Resources/test-resourcepool"
	})

	kubeClient := &testlib.FakeKubeClient{
		Infrastructure: infra,
		Nodes:          []*k8sv1.Node{node},
	}
	ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Ensure legacy path does not interfere
	if ctx.VMConfig.LegacyConfig != nil {
		ctx.VMConfig.LegacyConfig.Workspace.ResourcePoolPath = ""
	}
	// Ensure cluster permissions would be missing if checked (to prove infra read_only skips)
	setMissingClusterPermissionsAuthManager(t, ctx)

	// Get VM for node
	vCenter, err := GetVCenter(ctx, node)
	if err != nil {
		t.Fatalf("error getting vCenter for node %s: %s", node.Name, err)
	}
	vm, err := testlib.GetVM(vCenter.VMClient, node)
	if err != nil {
		t.Fatalf("error getting vm for node %s: %s", node.Name, err)
	}

	// Act
	err = check.CheckNode(ctx, node, vm)

	// Assert: should NOT error due to infra-based read-only
	assert.NoError(t, err)
}

func TestCheckComputeClusterPermissions_Infrastructure_NotReadOnly_DefaultRP(t *testing.T) {
	// Stage
	check := &CheckComputeClusterPermissions{}
	if err := check.StartCheck(); err != nil {
		t.Fatalf("StartCheck failed: %v", err)
	}

	// Node with region/zone labels matching the failure domain
	node := testlib.Node("DC0_H0_VM0", func(n *k8sv1.Node) {
		if n.Labels == nil {
			n.Labels = map[string]string{}
		}
		n.Labels["topology.kubernetes.io/region"] = "east"
		n.Labels["topology.kubernetes.io/zone"] = "east-1a"
	})

	infra := testlib.InfrastructureWithFailureDomain(func(inf *ocpv1.Infrastructure) {
		// Set ResourcePool to default cluster Resources (ends with /Resources) -> not read-only
		inf.Spec.PlatformSpec.VSphere.FailureDomains[0].Topology.ResourcePool = "/DC0/host/DC0_C0/Resources"
	})

	kubeClient := &testlib.FakeKubeClient{
		Infrastructure: infra,
		Nodes:          []*k8sv1.Node{node},
	}
	ctx, cleanup, err := SetupSimulator(kubeClient, testlib.DefaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Ensure legacy path does not interfere
	if ctx.VMConfig.LegacyConfig != nil {
		ctx.VMConfig.LegacyConfig.Workspace.ResourcePoolPath = ""
	}
	// Force missing cluster permissions so the non-read-only path surfaces an error
	setMissingClusterPermissionsAuthManager(t, ctx)

	// Get VM for node
	vCenter, err := GetVCenter(ctx, node)
	if err != nil {
		t.Fatalf("error getting vCenter for node %s: %s", node.Name, err)
	}
	vm, err := testlib.GetVM(vCenter.VMClient, node)
	if err != nil {
		t.Fatalf("error getting vm for node %s: %s", node.Name, err)
	}

	// Act
	err = check.CheckNode(ctx, node, vm)

	// Assert: expect no error when using custom ResourcePool
	assert.NoError(t, err)
}
