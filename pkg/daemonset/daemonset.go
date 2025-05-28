package daemonset

import (
	"context"
	"fmt"
	"os"
	"time"

	inferencev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
	clientset "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/instaslice-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/instaslice-operator/pkg/operator/constants"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/events"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
)

func RunDaemonset(ctx context.Context, cc *controllercmd.ControllerContext) error {
	// Get node name from environment
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		return fmt.Errorf("NODE_NAME environment variable is required")
	}

	// Check emulator mode
	emulatorMode := os.Getenv("EMULATOR_MODE") == "true"

	// Create Instaslice client
	instasliceClient, err := clientset.NewForConfig(cc.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create instaslice client: %w", err)
	}

	// Create Kubernetes client
	kubeClient, err := kubernetes.NewForConfig(cc.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create kube client: %w", err)
	}

	// Create informers
	kubeInformers := informers.NewSharedInformerFactory(kubeClient, 12*time.Hour)
	operatorConfigInformers := operatorclientinformers.NewSharedInformerFactory(instasliceClient, 12*time.Hour)

	// Create event recorder
	eventRecorder := events.NewKubeRecorder(kubeClient.CoreV1().Events(constants.InstaSliceOperatorNamespace),
		"instaslice-daemonset", &v1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Namespace:  constants.InstaSliceOperatorNamespace,
			Name:       "instaslice-daemonset",
		}, clock.RealClock{})

	// Create daemonset controller
	daemonsetController := NewDaemonsetController(&DaemonsetControllerConfig{
		KubeClient:         kubeClient,
		InstasliceClient:   instasliceClient,
		InstasliceInformer: operatorConfigInformers.OpenShiftOperator().V1alpha1().Instaslices().Informer(),
		NodeInformer:       kubeInformers.Core().V1().Nodes().Informer(),
		EventRecorder:      eventRecorder,
		NodeName:           nodeName,
		EmulatorMode:       emulatorMode,
	})

	// Start informers
	kubeInformers.Start(ctx.Done())
	operatorConfigInformers.Start(ctx.Done())

	// Wait for cache sync
	if !cache.WaitForCacheSync(ctx.Done(),
		kubeInformers.Core().V1().Nodes().Informer().HasSynced,
		operatorConfigInformers.OpenShiftOperator().V1alpha1().Instaslices().Informer().HasSynced) {
		return fmt.Errorf("failed to sync caches")
	}

	// Initialize GPU discovery in a separate goroutine
	go func() {
		if err := initializeGPUDiscovery(ctx, nodeName, emulatorMode, instasliceClient); err != nil {
			klog.ErrorS(err, "Failed to initialize GPU discovery")
		}
	}()

	// Run the controller
	go daemonsetController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}

// initializeGPUDiscovery handles initial GPU discovery and Instaslice object creation
func initializeGPUDiscovery(ctx context.Context, nodeName string, emulatorMode bool, client *clientset.Clientset) error {
	instaslice := &inferencev1alpha1.Instaslice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: constants.InstaSliceOperatorNamespace,
		},
	}

	// Create Instaslice object if it doesn't exist
	_, err := client.OpenShiftOperatorV1alpha1().Instaslices(constants.InstaSliceOperatorNamespace).Create(ctx, instaslice, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Instaslice object: %w", err)
	}

	if emulatorMode {
		return setupEmulatorMode(ctx, nodeName, client)
	}

	return setupRealMode(ctx, nodeName, client)
}

func setupEmulatorMode(ctx context.Context, nodeName string, client *clientset.Clientset) error {
	// TODO: Generate fake capacity for emulator mode
	klog.InfoS("Running in emulator mode", "node", nodeName)
	return nil
}

func setupRealMode(ctx context.Context, nodeName string, client *clientset.Clientset) error {
	klog.InfoS("Running in real mode, discovering GPUs", "node", nodeName)

	// Get the Instaslice object
	instaslice, err := client.OpenShiftOperatorV1alpha1().Instaslices(constants.InstaSliceOperatorNamespace).Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Instaslice object: %w", err)
	}

	// Create a temporary controller for discovery
	tempController := &DaemonsetController{
		nodeName:     nodeName,
		emulatorMode: false,
	}

	// Discover GPU profiles if not already done
	if instaslice.Status.NodeResources.NodeGPUs == nil {
		updatedInstaslice, _, _, err := tempController.discoverAvailableProfilesOnGpus(instaslice)
		if err != nil {
			return fmt.Errorf("failed to discover GPU profiles: %w", err)
		}

		// Update the Instaslice object with discovered profiles
		_, err = client.OpenShiftOperatorV1alpha1().Instaslices(constants.InstaSliceOperatorNamespace).UpdateStatus(ctx, updatedInstaslice, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update Instaslice status: %w", err)
		}
	}

	return nil
}
