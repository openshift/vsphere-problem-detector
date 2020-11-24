package operator

import (
	"context"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ check.KubeClient = &vSphereProblemDetectorController{}

// Implementation of check.KubeClient

func (c *vSphereProblemDetectorController) GetInfrastructure(ctx context.Context) (*ocpv1.Infrastructure, error) {
	return c.infraLister.Get(infrastructureName)
}

func (c *vSphereProblemDetectorController) ListNodes(ctx context.Context) ([]v1.Node, error) {
	list, err := c.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *vSphereProblemDetectorController) ListStorageClasses(ctx context.Context) ([]storagev1.StorageClass, error) {
	list, err := c.kubeClient.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *vSphereProblemDetectorController) ListPVs(ctx context.Context) ([]v1.PersistentVolume, error) {
	list, err := c.kubeClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
