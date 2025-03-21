package operator

import (
	"context"
	"errors"
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"
	applyoperatorv1 "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	operatorconfigclient "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	operatorclientinformers "github.com/openshift/client-go/operator/informers/externalversions"
	"github.com/openshift/library-go/pkg/apiserver/jsonpatch"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	operatorv1helpers "github.com/openshift/library-go/pkg/operator/v1helpers"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"
)

type OperatorClient struct {
	Informers operatorclientinformers.SharedInformerFactory
	Client    operatorconfigclient.StoragesGetter
}

var _ operatorv1helpers.OperatorClient = &OperatorClient{}

const (
	globalConfigName = "cluster"
)

func (c OperatorClient) Informer() cache.SharedIndexInformer {
	return c.Informers.Operator().V1().Storages().Informer()
}

func (c OperatorClient) GetOperatorState() (*operatorv1.OperatorSpec, *operatorv1.OperatorStatus, string, error) {
	instance, err := c.Informers.Operator().V1().Storages().Lister().Get(globalConfigName)
	if err != nil {
		return nil, nil, "", err
	}

	return &instance.Spec.OperatorSpec, &instance.Status.OperatorStatus, instance.ResourceVersion, nil
}

func (c OperatorClient) GetObjectMeta() (*metav1.ObjectMeta, error) {
	instance, err := c.Informers.Operator().V1().Storages().Lister().Get(globalConfigName)
	if err != nil {
		return nil, err
	}
	return &instance.ObjectMeta, nil
}

func (c OperatorClient) UpdateOperatorSpec(ctx context.Context, resourceVersion string, spec *operatorv1.OperatorSpec) (*operatorv1.OperatorSpec, string, error) {
	original, err := c.Informers.Operator().V1().Storages().Lister().Get(globalConfigName)
	if err != nil {
		return nil, "", err
	}
	copy := original.DeepCopy()
	copy.ResourceVersion = resourceVersion
	copy.Spec.OperatorSpec = *spec

	ret, err := c.Client.Storages().Update(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		return nil, "", err
	}

	return &ret.Spec.OperatorSpec, ret.ResourceVersion, nil
}

func (c OperatorClient) UpdateOperatorStatus(ctx context.Context, resourceVersion string, status *operatorv1.OperatorStatus) (*operatorv1.OperatorStatus, error) {
	original, err := c.Informers.Operator().V1().Storages().Lister().Get(globalConfigName)
	if err != nil {
		return nil, err
	}
	copy := original.DeepCopy()
	copy.ResourceVersion = resourceVersion
	copy.Status.OperatorStatus = *status

	ret, err := c.Client.Storages().UpdateStatus(ctx, copy, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	return &ret.Status.OperatorStatus, nil
}

func (c OperatorClient) GetOperatorInstance() (*operatorv1.Storage, error) {
	instance, err := c.Informers.Operator().V1().Storages().Lister().Get(globalConfigName)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func (c OperatorClient) GetOperatorStateWithQuorum(ctx context.Context) (*operatorv1.OperatorSpec, *operatorv1.OperatorStatus, string, error) {
	instance, err := c.Client.Storages().Get(ctx, globalConfigName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, "", err
	}

	return &instance.Spec.OperatorSpec, &instance.Status.OperatorStatus, instance.GetResourceVersion(), nil
}

func (c OperatorClient) ApplyOperatorSpec(ctx context.Context, fieldManager string, desiredSpecConfiguration *applyoperatorv1.OperatorSpecApplyConfiguration) error {
	if desiredSpecConfiguration == nil {
		return errors.New("desiredSpecConfiguration must have a value")
	}
	desiredSpecApplyConf := applyoperatorv1.Storage(globalConfigName).
		WithSpec(&applyoperatorv1.StorageSpecApplyConfiguration{
			OperatorSpecApplyConfiguration: *desiredSpecConfiguration})

	instance, err := c.GetOperatorInstance()
	switch {
	case apierrors.IsNotFound(err):
		// Object does not exist yet, proceed with apply to create it
	case err != nil:
		return fmt.Errorf("unable to get operator configuration: %w", err)
	default:
		original, err := applyoperatorv1.ExtractStorage(instance, fieldManager)
		if err != nil {
			return fmt.Errorf("unable to extract operator configuration: %w", err)
		}
		if original.Status != nil && equality.Semantic.DeepEqual(original, desiredSpecApplyConf) {
			return nil
		}
	}

	_, err = c.Client.Storages().Apply(ctx, desiredSpecApplyConf, metav1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	})

	if err != nil {
		return fmt.Errorf("unable to ApplySpec for operator using fieldManager %q: %w", fieldManager, err)
	}

	return nil
}

func (c OperatorClient) ApplyOperatorStatus(ctx context.Context, fieldManager string, desiredStatusConfiguration *applyoperatorv1.OperatorStatusApplyConfiguration) error {
	if desiredStatusConfiguration == nil {
		return errors.New("desiredStatusConfiguration must have a value")
	}

	desiredStatusApplyConf := applyoperatorv1.Storage(globalConfigName).
		WithStatus(&applyoperatorv1.StorageStatusApplyConfiguration{
			OperatorStatusApplyConfiguration: *desiredStatusConfiguration})

	instance, err := c.GetOperatorInstance()
	switch {
	case apierrors.IsNotFound(err):
		// Object does not exist yet, proceed with apply to create it
		v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desiredStatusApplyConf.Status.Conditions, nil)
	case err != nil:
		return fmt.Errorf("unable to get operator configuration: %w", err)
	default:
		original, err := applyoperatorv1.ExtractStorageStatus(instance, fieldManager)
		if err != nil {
			return fmt.Errorf("unable to extract operator configuration from status: %w", err)
		}
		if original.Status != nil && equality.Semantic.DeepEqual(original, desiredStatusApplyConf) {
			return nil
		}
		if original.Status != nil {
			v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desiredStatusApplyConf.Status.Conditions, original.Status.Conditions)
		} else {
			v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desiredStatusApplyConf.Status.Conditions, nil)
		}
	}

	_, err = c.Client.Storages().ApplyStatus(ctx, desiredStatusApplyConf, metav1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	})

	if err != nil {
		return fmt.Errorf("unable to ApplyStatus for operator using fieldManager %q: %w", fieldManager, err)
	}

	return nil
}

func (c OperatorClient) PatchOperatorStatus(ctx context.Context, jsonPatch *jsonpatch.PatchSet) error {
	jsonPatchData, err := jsonPatch.Marshal()
	if err != nil {
		return err
	}
	_, err = c.Client.Storages().Patch(ctx, globalConfigName, types.JSONPatchType, jsonPatchData, metav1.PatchOptions{}, "/status")
	return err
}
