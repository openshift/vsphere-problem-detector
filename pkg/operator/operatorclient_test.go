package operator

import (
	"context"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	applyoperatorv1 "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	fakeoperatorclient "github.com/openshift/client-go/operator/clientset/versioned/fake"
	operatorinformers "github.com/openshift/client-go/operator/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestApplyOperatorStatus_Success(t *testing.T) {
	ctx := context.Background()
	fieldManager := "test-manager"
	globalConfigName := "cluster"

	// Setup fake existing Storage object
	now := metav1.NewTime(time.Now())
	existing := &operatorv1.Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name: globalConfigName,
		},
		Status: operatorv1.StorageStatus{
			OperatorStatus: operatorv1.OperatorStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:               "Available",
						Status:             operatorv1.ConditionTrue,
						LastTransitionTime: now,
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)

	fakeClient := fakeoperatorclient.NewSimpleClientset(existing)
	storageClient := fakeClient.OperatorV1()
	informerFactory := operatorinformers.NewSharedInformerFactory(fakeClient, 0)
	storageInformer := informerFactory.Operator().V1().Storages().Informer()
	_ = storageInformer.GetStore().Add(existing)

	operatorClient := OperatorClient{
		Client:    storageClient,
		Informers: informerFactory,
	}

	// Define desired status with a new condition to force an update
	newCondition := operatorv1.OperatorCondition{
		Type:   "Degraded",
		Status: operatorv1.ConditionTrue,
	}

	desiredStatusConf := &applyoperatorv1.OperatorStatusApplyConfiguration{}
	desiredStatusConf.WithConditions(
		&applyoperatorv1.OperatorConditionApplyConfiguration{
			Type:               &newCondition.Type,
			Status:             &newCondition.Status,
			LastTransitionTime: &newCondition.LastTransitionTime,
			Reason:             &newCondition.Reason,
			Message:            &newCondition.Message,
		},
	)

	err := operatorClient.ApplyOperatorStatus(ctx, fieldManager, desiredStatusConf)
	if err != nil {
		t.Fatalf("ApplyOperatorStatus returned error: %v", err)
	}

	// Verify: Storage should be updated with new condition
	updated, err := fakeClient.OperatorV1().Storages().Get(ctx, globalConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get updated storage: %v", err)
	}

	found := false
	for _, cond := range updated.Status.Conditions {
		if cond.Type == "Degraded" && cond.Status == operatorv1.ConditionTrue {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected condition Degraded=True not found in updated status: %+v", updated.Status.Conditions)
	}
}

func TestApplyOperatorStatus_NilInput(t *testing.T) {
	client := OperatorClient{}
	err := client.ApplyOperatorStatus(context.Background(), "test-manager", nil)
	if err == nil || err.Error() != "desiredStatusConfiguration must have a value" {
		t.Errorf("expected error for nil configuration, got: %v", err)
	}
}

func TestApplyOperatorStatus_GetError(t *testing.T) {
	ctx := context.Background()

	// Setup fake client with no existing objects
	fakeClient := fakeoperatorclient.NewSimpleClientset()
	storageClient := fakeClient.OperatorV1()
	informerFactory := operatorinformers.NewSharedInformerFactory(fakeClient, 0)

	operatorClient := OperatorClient{
		Client:    storageClient,
		Informers: informerFactory,
	}

	// Define desired status with a new condition
	newCondition := operatorv1.OperatorCondition{
		Type:   "Processing",
		Status: operatorv1.ConditionTrue,
	}

	desiredStatusConf := &applyoperatorv1.OperatorStatusApplyConfiguration{}
	desiredStatusConf.WithConditions(
		&applyoperatorv1.OperatorConditionApplyConfiguration{
			Type:               &newCondition.Type,
			Status:             &newCondition.Status,
			LastTransitionTime: &newCondition.LastTransitionTime,
			Reason:             &newCondition.Reason,
			Message:            &newCondition.Message,
		},
	)

	err := operatorClient.ApplyOperatorStatus(ctx, "test-manager", desiredStatusConf)

	// Expect failed when object is not found
	if err == nil || strings.Contains(err.Error(), "storages\\.operator\\.openshift\\.io \"cluster\" not found") {
		t.Errorf("expected error for NotFound case, but got: %v", err)
	}
}
