package operator

import (
	"context"
	"fmt"
	"time"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	configinformer "github.com/openshift/client-go/config/informers/externalversions"
	operatorclient "github.com/openshift/client-go/operator/clientset/versioned"
	informer "github.com/openshift/client-go/operator/informers/externalversions"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/loglevel"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	resync               = 1 * time.Hour
	operatorName         = "vSphereProblemDetector"
	operatorNamespace    = "openshift-cluster-storage-operator"
	cloudConfigNamespace = "openshift-config"
)

var (
	RunningInVanillaKube = pflag.Bool("vanilla-kube", false, "Run in vanilla kube (default: false)")
	CloudConfigLocation  = pflag.String("cloud-config", "", "Location of vsphere cloud-configuration")
)

func RunOperator(ctx context.Context, controllerConfig *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(controllerConfig.ProtoKubeConfig)
	if err != nil {
		return err
	}

	if RunningInVanillaKube != nil && *RunningInVanillaKube {
		klog.Info("Starting the Informers for plain kubernetes.")
		operator := NewVSphereProblemDetectorControllerWithPlainKube(
			kubeClient,
			*CloudConfigLocation,
			controllerConfig.EventRecorder,
		)

		klog.Info("Starting the controllers for plain kube")
		go operator.Run(ctx, 1)
	} else {
		kubeInformers := v1helpers.NewKubeInformersForNamespaces(kubeClient, operatorNamespace, cloudConfigNamespace)

		operatorConfigClient, err := operatorclient.NewForConfig(controllerConfig.KubeConfig)
		if err != nil {
			return err
		}
		operatorConfigInformers := informer.NewSharedInformerFactoryWithOptions(operatorConfigClient, resync)

		operatorClient := &OperatorClient{
			operatorConfigInformers,
			operatorConfigClient.OperatorV1(),
		}

		configClient, err := configclient.NewForConfig(controllerConfig.KubeConfig)
		if err != nil {
			return err
		}
		configInformers := configinformer.NewSharedInformerFactoryWithOptions(configClient, resync)

		operator := NewVSphereProblemDetectorController(
			operatorClient,
			kubeClient,
			kubeInformers,
			configInformers.Config().V1().Infrastructures(),
			controllerConfig.EventRecorder,
		)

		logLevelController := loglevel.NewClusterOperatorLoggingController(operatorClient, controllerConfig.EventRecorder)

		klog.Info("Starting the Informers.")
		for _, informer := range []interface {
			Start(stopCh <-chan struct{})
		}{
			operatorConfigInformers,
			kubeInformers,
			configInformers,
		} {
			informer.Start(ctx.Done())
		}

		klog.Info("Starting the controllers")
		for _, controller := range []interface {
			Run(ctx context.Context, workers int)
		}{
			logLevelController,
			operator,
		} {
			go controller.Run(ctx, 1)
		}
	}
	<-ctx.Done()

	return fmt.Errorf("stopped")
}
