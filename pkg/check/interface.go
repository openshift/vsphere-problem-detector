package check

import (
	"context"
	"flag"
	"time"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/vmware/govmomi/vim25"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/legacy-cloud-providers/vsphere"
)

var (
	// Make the vSphere call timeout configurable.
	Timeout = flag.Duration("vmware-timeout", 10*time.Second, "Timeout of all VMware calls")

	// DefaultCheks is the list of all checks.
	DefaultChecks map[string]Check = map[string]Check{
		"CheckNodes": CheckNodes,
	}
)

// KubeClient is an interface between individual vSphere check and Kubernetes.
type KubeClient interface {
	// GetInfrastructure returns current Infrastructure instance.
	GetInfrastructure(ctx context.Context) (*ocpv1.Infrastructure, error)
	// ListNodes returns list of all nodes in the cluster.
	ListNodes(ctx context.Context) ([]v1.Node, error)
	// ListStorageClasses returns list of all storage classes in the cluster.
	ListStorageClasses(ctx context.Context) ([]storagev1.StorageClass, error)
	// ListPVs returns list of all PVs in the cluster.
	ListPVs(ctx context.Context) ([]v1.PersistentVolume, error)
}

// Interface of a single vSphere check. It gets connection to vSphere, vSphere config and connection to Kubernetes.
// It returns result of the check.
type Check func(ctx context.Context, vmConfig *vsphere.VSphereConfig, vmClient *vim25.Client, kubeClient KubeClient) error
