package operator

import (
	"context"

	ocpv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/vsphere-problem-detector/pkg/check"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ check.KubeClient = &vSphereProblemDetectorController{}

// Implementation of check.KubeClient

func (c *vSphereProblemDetectorController) GetInfrastructure(ctx context.Context) (*ocpv1.Infrastructure, error) {
	return c.infraLister.Get(infrastructureName)
}

func (c *vSphereProblemDetectorController) ListNodes(ctx context.Context) ([]*v1.Node, error) {
	return c.nodeLister.List(labels.Everything())
}

func (c *vSphereProblemDetectorController) ListStorageClasses(ctx context.Context) ([]*storagev1.StorageClass, error) {
	return c.scLister.List(labels.Everything())
}

func (c *vSphereProblemDetectorController) ListPVs(ctx context.Context) ([]*v1.PersistentVolume, error) {
	return c.pvLister.List(labels.Everything())
}
