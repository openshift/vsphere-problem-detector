package operator

import (
	"context"
	"fmt"
	"strings"

	ocpv1 "github.com/openshift/api/config/v1"
	operatorapi "github.com/openshift/api/operator/v1"
	infrainformer "github.com/openshift/client-go/config/informers/externalversions/config/v1"
	infralister "github.com/openshift/client-go/config/listers/config/v1"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	storagelister "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
)

const (
	migrationControllerName = "VSphereMigrationController"
	intreePluginName        = "kubernetes.io/vsphere-volume"
	annotationKeyName       = "storage.alpha.kubernetes.io/migrated-plugins"
)

type migrationDetectionController struct {
	operatorClient *OperatorClient
	kubeClient     kubernetes.Interface
	infraLister    infralister.InfrastructureLister
	csiNodeLister  storagelister.CSINodeLister
	eventRecorder  events.Recorder
}

func NewMigrationCheckController(operatorClient *OperatorClient,
	kubeClient kubernetes.Interface,
	namespacedInformer v1helpers.KubeInformersForNamespaces,
	configInformer infrainformer.InfrastructureInformer,
	eventRecorder events.Recorder) factory.Controller {

	csiNodeInformer := namespacedInformer.InformersFor("").Storage().V1().CSINodes()

	c := &migrationDetectionController{
		operatorClient: operatorClient,
		kubeClient:     kubeClient,
		infraLister:    configInformer.Lister(),
		csiNodeLister:  csiNodeInformer.Lister(),
		eventRecorder:  eventRecorder.WithComponentSuffix(controllerName),
	}
	return factory.New().WithSync(c.sync).WithSyncDegradedOnError(operatorClient).WithInformers(
		configInformer.Informer(),
		csiNodeInformer.Informer(),
	).ToController(migrationControllerName, c.eventRecorder)
}

func (c *migrationDetectionController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	klog.V(4).Infof("vSphereProblemDetectorController.Sync started")
	defer klog.V(4).Infof("vSphereProblemDetectorController.Sync finished")

	opSpec, _, _, err := c.operatorClient.GetOperatorState()
	if err != nil {
		return err
	}
	if opSpec.ManagementState != operatorapi.Managed {
		return nil
	}

	platformSupported, err := c.platformSupported()
	if err != nil {
		return err
	}

	if !platformSupported {
		return nil
	}

	storage, err := c.operatorClient.GetOperatorInstance()
	if err != nil {
		return err
	}

	if storage.Spec.VSphereStorageDriver != operatorapi.CSIWithMigrationDriver {
		return nil
	}

	// check if migration is complete
	migrationCompleteCondition := migrationControllerName + operatorapi.OperatorStatusTypeAvailable
	migrationInProgressCondition := migrationControllerName + operatorapi.OperatorStatusTypeProgressing

	if v1helpers.IsOperatorConditionTrue(storage.Status.Conditions, migrationCompleteCondition) {
		return nil
	}

	csiNodeObjects, err := c.csiNodeLister.List(labels.Everything())
	if err != nil {
		return err
	}

	migrationComplete := true

	for i := range csiNodeObjects {
		csiNode := csiNodeObjects[i]
		migratedDriversString, ok := csiNode.Annotations[annotationKeyName]
		if !ok {
			return fmt.Errorf("unexpected empty annotation for %s", annotationKeyName)
		}
		migratedDrivers := sets.NewString(strings.Split(migratedDriversString, ",")...)
		if !migratedDrivers.Has(intreePluginName) {
			migrationComplete = false
		}
	}

	updateFuncs := []v1helpers.UpdateStatusFunc{}
	availableCnd := operatorapi.OperatorCondition{
		Type:   migrationCompleteCondition,
		Status: operatorapi.ConditionFalse,
	}
	inProgressCond := operatorapi.OperatorCondition{
		Type:   migrationInProgressCondition,
		Status: operatorapi.ConditionTrue,
	}

	if migrationComplete {
		availableCnd.Status = operatorapi.ConditionTrue
		inProgressCond.Status = operatorapi.ConditionFalse
	} else {
		inProgressCond.Message = "Waiting for migration to finish"
	}
	updateFuncs = append(updateFuncs, v1helpers.UpdateConditionFn(availableCnd))
	updateFuncs = append(updateFuncs, v1helpers.UpdateConditionFn(inProgressCond))
	if _, _, updateErr := v1helpers.UpdateStatus(ctx, c.operatorClient, updateFuncs...); updateErr != nil {
		return updateErr
	}
	return nil
}

func (c *migrationDetectionController) platformSupported() (bool, error) {
	infra, err := c.infraLister.Get(infrastructureName)
	if err != nil {
		return false, err
	}

	if infra.Status.PlatformStatus == nil {
		klog.V(4).Infof("Unknown platform: infrastructure status.platformStatus is nil")
		return false, nil
	}
	if infra.Status.PlatformStatus.Type != ocpv1.VSpherePlatformType {
		klog.V(4).Infof("Unsupported platform: %s", infra.Status.PlatformStatus.Type)
		return false, nil
	}
	return true, nil
}
