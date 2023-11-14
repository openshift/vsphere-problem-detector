package check

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "zonalvolume",
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:       vSphereCSIDdriver,
							VolumeHandle: "blahblah",
						},
					},
					NodeAffinity: &v1.VolumeNodeAffinity{
						Required: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "topology.csi.vmware.com/openshift-zone",
											Operator: v1.NodeSelectorOperator("In"),
											Values:   []string{"us-west-1a"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ctx, cleanup, err := SetupSimulator(kubeClient, defaultModel)
	if err != nil {
		t.Fatalf("setupSimulator failed: %s", err)
	}
	defer cleanup()

	// Act
	rwxVolumeCount, zonalPVCount, err := countVolumeTypes(ctx)
	if err != nil {
		t.Errorf("Unexpected error : %+v", err)
	}
	if 1 != rwxVolumeCount {
		t.Errorf("expected 1 rwx pv, found %d", rwxVolumeCount)
	}

	if 1 != zonalPVCount {
		t.Errorf("expected 1 zonal pv, found %d", zonalPVCount)
	}
}
