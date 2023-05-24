package check

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/component-base/metrics/legacyregistry"
)

var (
	notSetNode1   = simulatedVM{"DC0_H0_VM0", "265104de-1472-547c-b873-6dc7883fb6cb"} //b4689bed-97f0-5bcd-8a4c-07477cc8f06f
	notSetNode2   = simulatedVM{"DC0_H0_VM1", "39365506-5a0a-5fd0-be10-9586ad53aaad"}
	notSetNode3   = simulatedVM{"DC0_C0_RP0_VM0", "cd0681bf-2f18-5c00-9b9b-8197c0095348"}
	disabledNode1 = simulatedVM{"DC0_C0_RP0_VM1", "f7c371d6-2003-5a48-9859-3bc9a8b08908"}
	disabledNode2 = simulatedVM{"DC0_C0_APP0_VM0", "bb58202a-c925-5cb6-b552-a8648ba3f1d5"}
	disabledNode3 = simulatedVM{"DC0_C0_APP0_VM1", "baf53482-475d-59dc-8777-2b193e7af804"}
	enabledNode1  = simulatedVM{"DC1_H0_VM0", "7930a567-e3b5-5e70-8d60-d32f5c963f60"}
	enabledNode2  = simulatedVM{"DC1_H0_VM1", "45412182-a62f-5c2f-bacb-4d3a45e2e5d9"}
	enabledNode3  = simulatedVM{"DC1_C0_RP0_VM0", "48cb9b41-18e2-5b40-8751-9f67e5cfbd87"}
)

// LIST OF NODES AND THEIR PROPERTY CONFIG
/*
152 - DC0_H0_VM0 - b4689bed-97f0-5bcd-8a4c-07477cc8f06f - NS
155 - DC0_H0_VM1 - 12f8928d-f144-5c57-89db-dd2d0902c9fa - NS
158 - DC0_C0_RP0_VM0 - bfff331f-7f07-572d-951e-edd3701dc061 - NS
161 - DC0_C0_RP0_VM1 - 6132d223-1566-5921-bc3b-df91ece09a4d - Disabled
164 - DC0_C0_APP0_VM0 - dd207ae0-7d81-5139-aa85-a7e5a292b005 - Disabled
167 - DC0_C0_APP0_VM1 - 7a6cc2df-3fc0-577a-ad08-6c0ec1525506- Disabled
170 - DC1_H0_VM0 - a628c174-5062-55cb-82be-c528a0b08c02 - Enabled
173 - DC1_H0_VM1 - e69f1292-4e60-540d-8182-ff2b19e4e97f - Enabled
176 - DC1_C0_RP0_VM0 - e9aeab08-6234-57ae-b312-929d3116064c - Enabled
*/
func TestVmCbtProperties(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	tests := []struct {
		name            string
		kubeClient      *fakeKubeClient
		expectError     bool
		error           string
		expectedMetrics string
	}{
		{
			name: "All Not Set",
			kubeClient: &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes: []*v1.Node{
					node(notSetNode1.name, withProviderID("vsphere://"+notSetNode1.uuid)),
					node(notSetNode2.name, withProviderID("vsphere://"+notSetNode2.uuid)),
					node(notSetNode3.name, withProviderID("vsphere://"+notSetNode3.uuid)),
				},
			},
			expectError: false,
			expectedMetrics: `
			# HELP vsphere_vm_cbt_checks [ALPHA] Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.
            # TYPE vsphere_vm_cbt_checks gauge
            vsphere_vm_cbt_checks{cbt="disabled"} 3
            vsphere_vm_cbt_checks{cbt="mismatch"} 0
`,
		},
		{
			name: "All Disabled",
			kubeClient: &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes: []*v1.Node{
					node(disabledNode1.name, withProviderID("vsphere://"+disabledNode1.uuid)),
					node(disabledNode2.name, withProviderID("vsphere://"+disabledNode2.uuid)),
					node(disabledNode3.name, withProviderID("vsphere://"+disabledNode3.uuid)),
				},
			},
			expectError: false,
			expectedMetrics: `
			# HELP vsphere_vm_cbt_checks [ALPHA] Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.
            # TYPE vsphere_vm_cbt_checks gauge
            vsphere_vm_cbt_checks{cbt="disabled"} 3
            vsphere_vm_cbt_checks{cbt="mismatch"} 0
`,
		},
		{
			name: "All Enabled",
			kubeClient: &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes: []*v1.Node{
					node(enabledNode1.name, withProviderID("vsphere://"+enabledNode1.uuid)),
					node(enabledNode2.name, withProviderID("vsphere://"+enabledNode2.uuid)),
					node(enabledNode3.name, withProviderID("vsphere://"+enabledNode3.uuid)),
				},
			},
			expectError: false,
			expectedMetrics: `
			# HELP vsphere_vm_cbt_checks [ALPHA] Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.
            # TYPE vsphere_vm_cbt_checks gauge
            vsphere_vm_cbt_checks{cbt="enabled"} 3
            vsphere_vm_cbt_checks{cbt="mismatch"} 0
`,
		},
		{
			name: "Mismatch",
			kubeClient: &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes: []*v1.Node{
					node(disabledNode1.name, withProviderID("vsphere://"+disabledNode1.uuid)),
					node(enabledNode2.name, withProviderID("vsphere://"+enabledNode2.uuid)),
					node(notSetNode3.name, withProviderID("vsphere://"+notSetNode3.uuid)),
				},
			},
			expectError: true,
			error:       "CBT Feature: All nodes are not matching in configuration.  The following have CBT disabled: [DC0_C0_RP0_VM1 DC0_C0_RP0_VM0]",
			expectedMetrics: `
			# HELP vsphere_vm_cbt_checks [ALPHA] Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.
            # TYPE vsphere_vm_cbt_checks gauge
            vsphere_vm_cbt_checks{cbt="disabled"} 2
            vsphere_vm_cbt_checks{cbt="enabled"} 1
            vsphere_vm_cbt_checks{cbt="mismatch"} 1
`,
		},
		{
			name: "One not set with others disabled",
			kubeClient: &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes: []*v1.Node{
					node(notSetNode1.name, withProviderID("vsphere://"+notSetNode1.uuid)),
					node(disabledNode2.name, withProviderID("vsphere://"+disabledNode2.uuid)),
					node(disabledNode3.name, withProviderID("vsphere://"+disabledNode3.uuid)),
				},
			},
			expectError: false,
			expectedMetrics: `
			# HELP vsphere_vm_cbt_checks [ALPHA] Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.
            # TYPE vsphere_vm_cbt_checks gauge
            vsphere_vm_cbt_checks{cbt="disabled"} 3
            vsphere_vm_cbt_checks{cbt="mismatch"} 0
`,
		},
		{
			name: "One not set with others enabled",
			kubeClient: &fakeKubeClient{
				infrastructure: infrastructure(),
				nodes: []*v1.Node{
					node(notSetNode1.name, withProviderID("vsphere://"+notSetNode1.uuid)),
					node(enabledNode2.name, withProviderID("vsphere://"+enabledNode2.uuid)),
					node(enabledNode3.name, withProviderID("vsphere://"+enabledNode3.uuid)),
				},
			},
			expectError: true,
			error:       "",
			expectedMetrics: `
			# HELP vsphere_vm_cbt_checks [ALPHA] Boolean metric based on whether ctkEnabled is consistent or not across all nodes in the cluster.
            # TYPE vsphere_vm_cbt_checks gauge
            vsphere_vm_cbt_checks{cbt="disabled"} 1
            vsphere_vm_cbt_checks{cbt="enabled"} 2
            vsphere_vm_cbt_checks{cbt="mismatch"} 1
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Stage
			check := CollectNodeCBT{
				lastMetricEmission: map[string]int{},
			}

			simctx, cleanup, err := setupSimulator(test.kubeClient, defaultModel)
			if err != nil {
				t.Fatalf("setupSimulator failed: %s", err)
			}
			defer cleanup()

			// Reset metrics from previous tests. Note: the tests can't run in parallel!
			legacyregistry.Reset()

			// Act - simulate loop through all nodes
			err = check.StartCheck()
			if err != nil {
				t.Errorf("StartCheck failed: %s", err)
			}

			// Run Test
			for _, node := range test.kubeClient.nodes {
				vm, err := getVirtualMachine(simctx, nil, node.Spec.ProviderID)
				if err != nil {
					t.Errorf("Error getting vm for node %s: %s", node.Name, err)
				}
				err = check.CheckNode(simctx, node, vm)
				if err != nil {
					t.Errorf("Unexpected error on node %s: %s", node.Name, err)
				}
			}

			check.FinishCheck(simctx)

			// Verify metrics
			if err := testutil.GatherAndCompare(legacyregistry.DefaultGatherer, strings.NewReader(test.expectedMetrics), "vsphere_vm_cbt_checks"); err != nil {
				t.Errorf("Unexpected metric: %s", err)
			}
		})
	}
}

// getVirtualMachine returns VirtualMachine based on provider ID passed in.  This will
// also load all properties related to VM using NodeProperties
func getVirtualMachine(ctx *CheckContext, dc *object.Datacenter, providerID string) (*mo.VirtualMachine, error) {
	tctx, cancel := context.WithTimeout(ctx.Context, *Timeout)
	defer cancel()

	s := object.NewSearchIndex(ctx.VMClient)
	vmUUID := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(providerID, "vsphere://")))
	svm, err := s.FindByUuid(tctx, dc, vmUUID, true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to find VM by UUID %s: %s", vmUUID, err)
	}
	if svm == nil {
		return nil, fmt.Errorf("unable to find VM by UUID %s", vmUUID)
	}

	// Load VM properties
	vm := object.NewVirtualMachine(ctx.VMClient, svm.Reference())

	var vmo mo.VirtualMachine
	err = vm.Properties(tctx, vm.Reference(), NodeProperties, &vmo)
	if err != nil {
		return nil, fmt.Errorf("failed to load properties for VM %s: %s", vmUUID, err)
	}

	return &vmo, nil
}
