package check

import (
	"context"
	"flag"
	"github.com/vmware/govmomi/object"
	vapitags "github.com/vmware/govmomi/vapi/tags"
	"time"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/util"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/legacy-cloud-providers/vsphere"
)

// TagManager defines an interface to an implementation of the AuthorizationManager to facilitate mocking.
type TagManager interface {
	AttachTag(ctx context.Context, tagID string, ref mo.Reference) error
	CreateCategory(ctx context.Context, category *vapitags.Category) (string, error)
	CreateTag(ctx context.Context, tag *vapitags.Tag) (string, error)
	DeleteCategory(ctx context.Context, category *vapitags.Category) error
	DeleteTag(ctx context.Context, tag *vapitags.Tag) error
	DetachTag(ctx context.Context, tagID string, ref mo.Reference) error
	ListCategories(ctx context.Context) ([]string, error)
	ListTags(ctx context.Context) ([]string, error)
	GetCategories(ctx context.Context) ([]vapitags.Category, error)
	GetCategory(ctx context.Context, id string) (*vapitags.Category, error)
	GetTagsForCategory(ctx context.Context, id string) ([]vapitags.Tag, error)
	GetAttachedTags(ctx context.Context, ref mo.Reference) ([]vapitags.Tag, error)
	GetAttachedTagsOnObjects(ctx context.Context, objectID []mo.Reference) ([]vapitags.AttachedTags, error)
	GetAttachedObjectsOnTags(ctx context.Context, tagID []string) ([]vapitags.AttachedObjects, error)
}

// Finder interface represents the client that is used to connect to VSphere to get specific
// information from the resources in the VCenter. This interface just describes all the useful
// functions used by the vsphere problem detector from the finder function in vmware govmomi
// package and is mostly used to create a mock client that can be used for testing.
type Finder interface {
	Datacenter(ctx context.Context, path string) (*object.Datacenter, error)
	DatacenterList(ctx context.Context, path string) ([]*object.Datacenter, error)
	DatastoreList(ctx context.Context, path string) ([]*object.Datastore, error)
	ClusterComputeResource(ctx context.Context, path string) (*object.ClusterComputeResource, error)
	ClusterComputeResourceList(ctx context.Context, path string) ([]*object.ClusterComputeResource, error)
	Folder(ctx context.Context, path string) (*object.Folder, error)
	NetworkList(ctx context.Context, path string) ([]object.NetworkReference, error)
	Network(ctx context.Context, path string) (object.NetworkReference, error)
	ResourcePool(ctx context.Context, path string) (*object.ResourcePool, error)
}

var (
	// Make the vSphere call timeout configurable.
	Timeout = flag.Duration("vmware-timeout", 5*time.Minute, "Timeout of all VMware calls")

	// DefaultClusterChecks is the list of all checks.
	DefaultClusterChecks map[string]ClusterCheck = map[string]ClusterCheck{
		"CheckTaskPermissions":    CheckTaskPermissions,
		"ClusterInfo":             CollectClusterInfo,
		"CheckFolderPermissions":  CheckFolderPermissions,
		"CheckDefaultDatastore":   CheckDefaultDatastore,
		"CheckStorageClasses":     CheckStorageClasses,
		"CountVolumeTypes":        CountPVTypes,
		"CheckAccountPermissions": CheckAccountPermissions,
		"CheckZoneTags":           CheckZoneTags,
	}
	DefaultNodeChecks []NodeCheck = []NodeCheck{
		&CheckNodeDiskUUID{},
		&CheckNodeProviderID{},
		&CollectNodeHWVersion{
			lastMetricEmission: map[string]int{},
		},
		&CollectNodeESXiVersion{
			lastMetricEmission: make(map[[2]string]int),
		},
		&CheckNodeDiskPerf{},
		&CheckComputeClusterPermissions{},
		&CheckResourcePoolPermissions{},
	}

	// NodeProperties is a list of properties that NodeCheck can rely on to be pre-filled.
	// Add a property to this list when a NodeCheck uses it.
	NodeProperties = []string{"config.extraConfig", "config.flags", "config.version", "runtime.host"}
)

// KubeClient is an interface between individual vSphere check and Kubernetes.
type KubeClient interface {
	// GetInfrastructure returns current Infrastructure instance.
	GetInfrastructure(ctx context.Context) (*ocpv1.Infrastructure, error)
	// ListNodes returns list of all nodes in the cluster.
	ListNodes(ctx context.Context) ([]*v1.Node, error)
	// ListStorageClasses returns list of all storage classes in the cluster.
	ListStorageClasses(ctx context.Context) ([]*storagev1.StorageClass, error)
	// ListPVs returns list of all PVs in the cluster.
	ListPVs(ctx context.Context) ([]*v1.PersistentVolume, error)
}

type CheckContext struct {
	Context     context.Context
	VMConfig    *vsphere.VSphereConfig
	VMClient    *vim25.Client
	TagManager  TagManager
	Username    string
	AuthManager AuthManager
	KubeClient  KubeClient
	ClusterInfo *util.ClusterInfo
}

// Interface of a single vSphere cluster-level check. It gets connection to vSphere, vSphere config and connection to Kubernetes.
// It returns result of the check.
type ClusterCheck func(ctx *CheckContext) error

// Interface of a single vSphere node-level check.
// Reason for separate node-level checks:
// 1) We want to expose per node metrics what checks failed/succeeded.
// 2) When multiple checks need a VM, we want to get it only once from the vSphere API.
//
// Every round of checks starts with a single StartCheck(), then CheckNode for each
// node (possibly in parallel). After all CheckNodes finish, the FinishCheck() is called.
// It is guaranteed that only one "round" is running at the time.
type NodeCheck interface {
	Name() string
	// Start new round of checks. The check may initialize its internal state here.
	StartCheck() error
	// Check of a single node. It gets connection to vSphere, vSphere config, connection
	// to Kubernetes and a node to check. Returns result of the check.
	// Multiple CheckNodes can run in parallel, each for a different node!
	CheckNode(ctx *CheckContext, node *v1.Node, vm *mo.VirtualMachine) error
	// Finish current round of checks. The check may report metrics here.
	// Since it's called serially for all checks, it should not perform too
	// expensive calculations. In addition, it must respect ctx.Context.
	// It will be called after all CheckNode calls finish.
	FinishCheck(ctx *CheckContext)
}
