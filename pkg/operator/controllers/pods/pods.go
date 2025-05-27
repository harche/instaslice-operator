package pods

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

// PodController watches Pod resources and enqueues them for processing.
type PodController struct {
	namespace     string
	kubeClient    kubernetes.Interface
	podInformer   cache.SharedInformer
	eventRecorder events.Recorder
}

// PodControllerConfig holds the configuration for PodController.
type PodControllerConfig struct {
	Namespace     string
	KubeClient    kubernetes.Interface
	PodInformer   cache.SharedInformer
	EventRecorder events.Recorder
}

// NewPodController creates a new controller that watches Pods in a namespace.
func NewPodController(config *PodControllerConfig) factory.Controller {
	c := &PodController{
		namespace:     config.Namespace,
		kubeClient:    config.KubeClient,
		podInformer:   config.PodInformer,
		eventRecorder: config.EventRecorder,
	}

	return factory.New().
		WithSync(c.sync).
		WithInformersQueueKeysFunc(c.nameToKey, c.podInformer).
		ToController("PodController", c.eventRecorder)
}

func (c *PodController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	podName := syncCtx.QueueKey()
	klog.V(2).InfoS("Pod Sync", "queue_key", podName)

	pod, err := c.kubeClient.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	klog.V(2).InfoS("Pod", "pod", pod)
	return nil
}

// nameToKey returns the key (name) for a Pod object to enqueue.
func (c *PodController) nameToKey(obj runtime.Object) []string {
	metaObj, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		klog.Errorf("the object is not a metav1.ObjectMetaAccessor: %T", obj)
		return []string{}
	}
	return []string{metaObj.GetObjectMeta().GetName()}
}
