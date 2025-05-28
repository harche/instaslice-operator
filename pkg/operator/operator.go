package operator

import (
   "context"
   "os"
   "time"

   "github.com/openshift/library-go/pkg/controller/controllercmd"
   "github.com/openshift/library-go/pkg/operator/loglevel"
   "github.com/openshift/library-go/pkg/operator/v1helpers"

   apiextclientv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
   "k8s.io/client-go/discovery"
   "k8s.io/client-go/dynamic"
   "k8s.io/client-go/informers"
   "k8s.io/client-go/kubernetes"
   appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
   metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
   "k8s.io/klog/v2"

	resourcecache "github.com/openshift/instaslice-operator/pkg/operator/cache"
	operatorconfigclient "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/instaslice-operator/pkg/generated/informers/externalversions"
	instaslicecontroller "github.com/openshift/instaslice-operator/pkg/operator/controllers/instaslice"
	podcontroller "github.com/openshift/instaslice-operator/pkg/operator/controllers/pod"
	"github.com/openshift/instaslice-operator/pkg/operator/operatorclient"
)

var operatorNamespace = "instaslice-system"

func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}

	apiextensionClient, err := apiextclientv1.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}

	appsClient, err := appsv1client.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}

	operatorConfigClient, err := operatorconfigclient.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}
	operatorConfigInformers := operatorclientinformers.NewSharedInformerFactory(operatorConfigClient, 10*time.Minute)

	namespace := cc.OperatorNamespace
	if namespace == "openshift-config-managed" {
		// we need to fall back to our default namespace rather than library-go's when running outside the cluster
		namespace = operatorNamespace
	}

	// KubeInformer for Instaslice namespace
	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient, "", namespace)

	instasliceClient := &operatorclient.InstasliceOperatorSetClient{
		Ctx:               ctx,
		SharedInformer:    operatorConfigInformers.OpenShiftOperator().V1alpha1().InstasliceOperators().Informer(),
		Lister:            operatorConfigInformers.OpenShiftOperator().V1alpha1().InstasliceOperators().Lister(),
		OperatorClient:    operatorConfigClient.OpenShiftOperatorV1alpha1(),
		OperatorNamespace: namespace,
	}

	targetConfigReconciler := NewTargetConfigReconciler(
		os.Getenv("RELATED_IMAGE_DAEMONSET_IMAGE"),
		os.Getenv("RELATED_IMAGE_WEBHOOK_IMAGE"),
		namespace,
		operatorConfigClient.OpenShiftOperatorV1alpha1().InstasliceOperators(namespace),
		operatorConfigInformers.OpenShiftOperator().V1alpha1().InstasliceOperators(),
		kubeInformersForNamespaces,
		appsClient,
		instasliceClient,
		dynamicClient,
		discoveryClient,
		kubeClient,
		apiextensionClient,
		cc.EventRecorder,
	)

	// Create the log controller
	logLevelController := loglevel.NewClusterOperatorLoggingController(instasliceClient, cc.EventRecorder)

	// Create the Instaslice Controller
	sliceControllerConfig := instaslicecontroller.InstasliceControllerConfig{
		Namespace:          namespace,
		OperatorClient:     operatorConfigClient,
		KubeClient:         kubeClient,
		InstasliceInformer: operatorConfigInformers.OpenShiftOperator().V1alpha1().Instaslices().Informer(),
		EventRecorder:      cc.EventRecorder,
	}
	instasliceController := instaslicecontroller.NewInstasliceController(&sliceControllerConfig)
	// Start resource tracker for node resources
	klog.InfoS("Starting ResourceTracker")
	resourceTracker, err := resourcecache.NewResourceTracker(cc.KubeConfig)
	if err != nil {
		return err
	}
	go func() {
		if err := resourceTracker.Start(ctx); err != nil {
			klog.ErrorS(err, "ResourceTracker exited")
		}
	}()

	// Create and start Pod Controller
	klog.InfoS("Starting Pod Controller")
	// Create a filtered Pod informer to only watch pods labeled by instaslice webhook
	filteredPodInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		10*time.Minute,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = "instaslice.com/pod=true"
		}),
	)
	podInformer := filteredPodInformerFactory.Core().V1().Pods().Informer()
	instasliceInformer := operatorConfigInformers.OpenShiftOperator().V1alpha1().Instaslices().Informer()
	podController := podcontroller.NewPodController(&podcontroller.PodControllerConfig{
		KubeClient:         kubeClient,
		InstasliceClient:   operatorConfigClient,
		PodInformer:        podInformer,
		InstasliceInformer: instasliceInformer,
		ResourceCache:      resourceTracker.Cache(),
		EventRecorder:      cc.EventRecorder,
	})
	go podController.Run(ctx, 1)

	// Create webhook server
	// _, err = webhookserver.NewServer(cc.ProtoKubeConfig, "", "", "")
	// if err != nil {
	// 	return err
	// }

	klog.Infof("Starting informers")
	operatorConfigInformers.Start(ctx.Done())
	// Start namespace informers
	kubeInformersForNamespaces.Start(ctx.Done())
	// Start filtered Pod informer
	filteredPodInformerFactory.Start(ctx.Done())

	klog.Infof("Starting log level controller")
	go logLevelController.Run(ctx, 1)
	klog.Infof("Starting target config reconciler")
	go targetConfigReconciler.Run(ctx, 1)
	klog.Infof("Starting Instaslice Controller")
	go instasliceController.Run(ctx, 1)
	klog.Infof("Starting Webhook Server")
	// go mutatingWebhookServer.Run(ctx)

	<-ctx.Done()

	return nil
}
