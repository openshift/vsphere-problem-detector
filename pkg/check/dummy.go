package check

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vim25"
	"k8s.io/legacy-cloud-providers/vsphere"
)

// CheckDummy is a dummy check that always fails to test the operator & its exp. backoff.
func CheckDummy(ctx context.Context, vmConfig *vsphere.VSphereConfig, vmClient *vim25.Client, kubeClient KubeClient) error {
	return fmt.Errorf("Dummy error")
}
