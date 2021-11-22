package check

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestCountRWXVolumes(t *testing.T) {
	kubeClient := &fakeKubeClient{
		infrastructure: infrastructure(),
		nodes:          defaultNodes(),
		pvs: []*v1.PersistentVolume{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foobar",
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:       vSphereCSIDdriver,
							VolumeHandle: "file:blahblah",
						},
					},
				},
			},
		},
	}

	ctx, cleanup, err := setupSimulator(kubeClient, defaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Act
	count, err := countRWXPVsFromCluster(ctx)
	if err != nil {
		t.Errorf("Unexpected error : %+v", err)
	}
	if 1 != count {
		t.Errorf("expected 1 rwx pv, found %d", count)
	}
}
