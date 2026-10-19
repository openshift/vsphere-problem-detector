package operator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	corelister "k8s.io/client-go/listers/core/v1"
	k8scache "k8s.io/client-go/tools/cache"

	"github.com/openshift/vsphere-problem-detector/pkg/cache"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	"github.com/openshift/vsphere-problem-detector/pkg/testlib"
)

func TestIsVSphereNode(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		expected   bool
	}{
		{"vSphere providerID", "vsphere://265104de-1472-547c-b873-6dc7883fb6cb", true},
		{"empty providerID (e.g. bare metal node)", "", false},
		{"other cloud provider", "aws://i-0123456789abcdef0", false},
		{"bare UUID without prefix", "265104de-1472-547c-b873-6dc7883fb6cb", false},
		{"wrong prefix case", "VSphere://265104de-1472-547c-b873-6dc7883fb6cb", false},
		{"single slash", "vsphere:/265104de-1472-547c-b873-6dc7883fb6cb", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := testlib.Node("test-node", testlib.WithProviderID(test.providerID))
			if got := isVSphereNode(node); got != test.expected {
				t.Errorf("isVSphereNode(node with providerID %q) = %t, expected %t", test.providerID, got, test.expected)
			}
		})
	}
}

// recordingNodeCheck records the names of all nodes it checked.
type recordingNodeCheck struct {
	mutex sync.Mutex
	nodes []string
}

var _ check.NodeCheck = &recordingNodeCheck{}

func (c *recordingNodeCheck) Name() string { return "RecordingCheck" }

func (c *recordingNodeCheck) StartCheck() error { return nil }

func (c *recordingNodeCheck) CheckNode(ctx *check.CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.nodes = append(c.nodes, node.Name)
	return nil
}

func (c *recordingNodeCheck) FinishCheck(ctx *check.CheckContext) {}

// failingNodeCheck fails for every node it checks, including the node name in the error.
type failingNodeCheck struct{}

var _ check.NodeCheck = &failingNodeCheck{}

func (c *failingNodeCheck) Name() string { return "FailingCheck" }

func (c *failingNodeCheck) StartCheck() error { return nil }

func (c *failingNodeCheck) CheckNode(ctx *check.CheckContext, node *v1.Node, vm *mo.VirtualMachine) error {
	return fmt.Errorf("fake check failure on node %s", node.Name)
}

func (c *failingNodeCheck) FinishCheck(ctx *check.CheckContext) {}

// TestEnqueueNodeChecksSkipsNonVSphereNodes checks that node checks are
// enqueued only for nodes that are vSphere VMs. Nodes added from other
// platforms (e.g. bare metal compute nodes in a mixed cluster) must be
// skipped without producing any failed check findings, while checks for
// vSphere nodes keep running and reporting genuine failures.
// https://issues.redhat.com/browse/OCPBUGS-130729
func TestEnqueueNodeChecksSkipsNonVSphereNodes(t *testing.T) {
	vSphereNodes := testlib.DefaultNodes()
	bareMetalNode := testlib.Node("baremetal-0") // no providerID
	otherCloudNode := testlib.Node("other-cloud-0", testlib.WithProviderID("aws://i-0123456789abcdef0"))
	nodes := append(append([]*v1.Node{}, vSphereNodes...), bareMetalNode, otherCloudNode)

	kubeClient := &testlib.FakeKubeClient{Nodes: nodes}
	checkContext, cleanup, err := setupCheckContext(kubeClient)
	if err != nil {
		t.Fatalf("failed to set up simulator: %s", err)
	}
	defer cleanup()

	indexer := k8scache.NewIndexer(k8scache.MetaNamespaceKeyFunc, k8scache.Indexers{})
	for i := range nodes {
		if err := indexer.Add(nodes[i]); err != nil {
			t.Fatalf("failed to add node %s to indexer: %s", nodes[i].Name, err)
		}
	}

	recorder := &recordingNodeCheck{}
	failer := &failingNodeCheck{}
	controller := &vSphereProblemDetectorController{
		nodeLister: corelister.NewNodeLister(indexer),
		nodeChecks: []check.NodeCheck{recorder, failer},
	}
	checker := &vSphereChecker{controller: controller}

	checkRunner := NewCheckThreadPool(parallelVSPhereCalls, channelBufferSize)
	resultCollector := NewResultsCollector()

	if err := checker.enqueueNodeChecks(checkContext, checkRunner, resultCollector); err != nil {
		t.Fatalf("enqueueNodeChecks failed: %s", err)
	}
	if err := checkRunner.Wait(checkContext.Context); err != nil {
		t.Fatalf("failed to wait for checks: %s", err)
	}
	results, aggregateErr := resultCollector.Collect()

	// Checks must run for all vSphere nodes...
	checkedNodes := append([]string{}, recorder.nodes...)
	sort.Strings(checkedNodes)
	expectedNodes := []string{}
	for i := range vSphereNodes {
		expectedNodes = append(expectedNodes, vSphereNodes[i].Name)
	}
	sort.Strings(expectedNodes)
	if !reflect.DeepEqual(checkedNodes, expectedNodes) {
		t.Errorf("expected checks to run only for vSphere nodes %v, got %v", expectedNodes, checkedNodes)
	}

	// ...including the always-failing check, so genuine findings stay visible.
	for _, name := range expectedNodes {
		found := false
		for _, res := range results {
			if res.Name == failer.Name() && res.Error != nil && strings.Contains(res.Error.Error(), name) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected a failure of %s for vSphere node %s, got none", failer.Name(), name)
		}
	}

	// Non-vSphere nodes must not produce any failed check findings
	// (such as "VMUUID is not set for node ...").
	for _, skipped := range []string{bareMetalNode.Name, otherCloudNode.Name} {
		for _, res := range results {
			if res.Error != nil && strings.Contains(res.Error.Error(), skipped) {
				t.Errorf("found check result mentioning skipped node %s: %s", skipped, res.Error)
			}
		}
		if aggregateErr != nil && strings.Contains(aggregateErr.Error(), skipped) {
			t.Errorf("aggregate error mentions skipped node %s: %s", skipped, aggregateErr)
		}
	}
}

// setupCheckContext creates a CheckContext connected to a vCenter simulator.
func setupCheckContext(kubeClient *testlib.FakeKubeClient) (*check.CheckContext, func(), error) {
	setup, cleanup, err := testlib.SetupSimulatorWithConfig(kubeClient, testlib.DefaultModel, "")
	if err != nil {
		return nil, nil, err
	}

	vcMap := make(map[string]*check.VCenter)
	for vCenterName := range setup.VMConfig.Config.VirtualCenter {
		vc := check.VCenter{
			VCenterName: vCenterName,
			TagManager:  setup.VCenters[vCenterName].TagManager,
			VMClient:    setup.VCenters[vCenterName].VMClient,
			Cache:       cache.NewCheckCache(setup.VCenters[vCenterName].VMClient),
			Username:    setup.VCenters[vCenterName].Username,
			Model:       setup.VCenters[vCenterName].Model,
		}
		vcMap[vCenterName] = &vc
	}

	checkContext := &check.CheckContext{
		Context:     setup.Context,
		VMConfig:    setup.VMConfig,
		ClusterInfo: setup.ClusterInfo,
		KubeClient:  kubeClient,
		VCenters:    vcMap,
	}
	return checkContext, cleanup, nil
}
