package pod

import (
	"context"
	"fmt"
	"time"

	resourcecache "github.com/openshift/instaslice-operator/pkg/operator/cache"
	inferencev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
	clientset "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned"
	"github.com/openshift/instaslice-operator/pkg/operator/constants"
	"github.com/openshift/instaslice-operator/pkg/operator/utils"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type PodController struct {
	kubeClient         *kubernetes.Clientset
	instasliceClient   *clientset.Clientset
	podInformer        cache.SharedInformer
	instasliceInformer cache.SharedInformer
	resourceCache      *resourcecache.ResourceCache
	eventRecorder      events.Recorder
}

type PodControllerConfig struct {
	KubeClient         *kubernetes.Clientset
	InstasliceClient   *clientset.Clientset
	PodInformer        cache.SharedInformer
	InstasliceInformer cache.SharedInformer
	ResourceCache      *resourcecache.ResourceCache
	EventRecorder      events.Recorder
}

func NewPodController(config *PodControllerConfig) factory.Controller {
	c := &PodController{
		kubeClient:         config.KubeClient,
		instasliceClient:   config.InstasliceClient,
		podInformer:        config.PodInformer,
		instasliceInformer: config.InstasliceInformer,
		resourceCache:      config.ResourceCache,
		eventRecorder:      config.EventRecorder,
	}

	return factory.New().
		WithSync(c.sync).
		WithInformersQueueKeysFunc(c.nameToKey, c.podInformer).
		ToController("PodController", c.eventRecorder)
}

func (c *PodController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(syncCtx.QueueKey())
	if err != nil {
		return err
	}

	klog.V(2).InfoS("Pod Sync", "namespace", namespace, "name", name)

	pod, err := c.kubeClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		klog.ErrorS(err, "unable to fetch pod", "namespace", namespace, "name", name)
		return err
	}

	// Skip pods with scheduling gates other than InstaSlice gate
	if utils.IsPodGatedByOthers(pod) {
		return nil
	}

	isPodGated := utils.CheckIfPodGatedByInstaSlice(pod)

	// Skip pods that are not gated by InstaSlice and don't have finalizer
	if !isPodGated && !utils.ContainsFinalizer(pod, constants.FinalizerName) {
		return nil
	}

	// Add finalizer to pods gated by InstaSlice
	if isPodGated && !utils.ContainsFinalizer(pod, constants.FinalizerName) {
		pod.Finalizers = append(pod.Finalizers, constants.FinalizerName)
		_, err := c.kubeClient.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
		if err != nil {
			klog.ErrorS(err, "failed to add finalizer to pod", "namespace", namespace, "name", name)
			return err
		}
	}

	// Handle failed pods - remove finalizer so user can delete
	if pod.Status.Phase == v1.PodFailed && utils.ContainsFinalizer(pod, constants.FinalizerName) {
		return c.handleFailedPod(ctx, pod)
	}

	// Handle succeeded pods
	if pod.Status.Phase == v1.PodSucceeded && utils.ContainsFinalizer(pod, constants.FinalizerName) {
		return c.handleSucceededPod(ctx, pod)
	}

	// Handle deleted pods that are still gated
	if !pod.DeletionTimestamp.IsZero() && isPodGated {
		return c.handleDeletedGatedPod(ctx, pod)
	}

	// Handle graceful termination
	if !pod.DeletionTimestamp.IsZero() {
		return c.handleGracefulTermination(ctx, pod)
	}

	// Handle allocation logic for gated pods
	if isPodGated {
		return c.handleGatedPod(ctx, pod)
	}

	return nil
}

func (c *PodController) handleFailedPod(ctx context.Context, pod *v1.Pod) error {
	// TODO: Implement failed pod handling logic
	// This should coordinate with Instaslice controller to clean up allocations
	klog.V(2).InfoS("Handling failed pod", "pod", pod.Name)
	return nil
}

func (c *PodController) handleSucceededPod(ctx context.Context, pod *v1.Pod) error {
	// TODO: Implement succeeded pod handling logic
	// This should coordinate with Instaslice controller to clean up allocations
	klog.V(2).InfoS("Handling succeeded pod", "pod", pod.Name)
	return nil
}

func (c *PodController) handleDeletedGatedPod(ctx context.Context, pod *v1.Pod) error {
	// TODO: Implement deleted gated pod handling logic
	klog.V(2).InfoS("Handling deleted gated pod", "pod", pod.Name)
	return nil
}

func (c *PodController) handleGracefulTermination(ctx context.Context, pod *v1.Pod) error {
	// TODO: Implement graceful termination logic
	klog.V(2).InfoS("Handling graceful termination", "pod", pod.Name)

	if utils.ContainsFinalizer(pod, constants.FinalizerName) {
		elapsed := time.Since(pod.DeletionTimestamp.Time)
		if elapsed > 30*time.Second {
			// Force cleanup after 30 seconds
			return c.setAllocationToDeleting(ctx, pod)
		}
		// Wait for remaining time
		remainingTime := 30*time.Second - elapsed
		klog.V(2).InfoS("Waiting for graceful termination", "pod", pod.Name, "remaining", remainingTime)
		return factory.SyntheticRequeueError
	}
	return nil
}

func (c *PodController) handleGatedPod(ctx context.Context, pod *v1.Pod) error {
	klog.V(2).InfoS("Handling gated pod", "pod", pod.Name)
	if len(pod.Spec.Containers) == 0 {
		return fmt.Errorf("no container found inside pod: %s", pod.Name)
	}
	if len(pod.Spec.Containers) != 1 {
		return fmt.Errorf("multiple containers in pod not supported: %s", pod.Name)
	}
	if !utils.HasMIGResource(pod) {
		return nil
	}
	limits := pod.Spec.Containers[0].Resources.Limits
	profileName := utils.ExtractProfileName(limits)
	if profileName == "" {
		return fmt.Errorf("unable to extract profile name from pod resources: %s", pod.Name)
	}
	// Iterate Instaslice objects to find a fitting node and device
	for _, obj := range c.instasliceInformer.GetStore().List() {
		slice, ok := obj.(*inferencev1alpha1.Instaslice)
		if !ok {
			continue
		}
		if !c.resourceCache.Fits(slice.Name, pod) {
			continue
		}
		allocReq, allocRes, err := findNodeAndDeviceForSlice(ctx, slice, profileName, &utils.FirstFitPolicy{}, pod)
		if err != nil {
			klog.ErrorS(err, "failed to find placement", "pod", pod.Name, "slice", slice.Name)
			continue
		}
		if err := utils.UpdateInstasliceAllocationWithClientset(ctx, c.instasliceClient, slice.Name, constants.InstaSliceOperatorNamespace, allocRes, allocReq); err != nil {
			klog.ErrorS(err, "failed to update instaslice allocation", "pod", pod.Name, "slice", slice.Name)
			return err
		}
		return nil
	}
	klog.InfoS("No suitable instaslice found for pod", "pod", pod.Name)
	return nil
}

func (c *PodController) setAllocationToDeleting(ctx context.Context, pod *v1.Pod) error {
	// TODO: Implement allocation deletion logic
	klog.V(2).InfoS("Setting allocation to deleting", "pod", pod.Name)
	return nil
}

func (c *PodController) removeFinalizer(ctx context.Context, pod *v1.Pod) error {
	if utils.RemoveFinalizer(pod, constants.FinalizerName) {
		_, err := c.kubeClient.CoreV1().Pods(pod.Namespace).Update(ctx, pod, metav1.UpdateOptions{})
		if err != nil {
			klog.ErrorS(err, "unable to update removal of finalizer", "pod", pod.Name)
			return err
		}
		klog.InfoS("finalizer removed", "pod", pod.Name)
	}
	return nil
}

// findNodeAndDeviceForSlice selects a placement for the pod on the given Instaslice
func findNodeAndDeviceForSlice(ctx context.Context, slice *inferencev1alpha1.Instaslice, profileName string, policy utils.AllocationPolicy, pod *v1.Pod) (*inferencev1alpha1.AllocationRequest, *inferencev1alpha1.AllocationResult, error) {
	klog.V(2).InfoS("Finding node and device for slice", "profile", profileName, "pod", pod.Name)
	size, giProfile, ciProfileID, ciEngProfileID := extractGpuProfile(slice, profileName)
	if size == 0 {
		return nil, nil, fmt.Errorf("profile %s not found on node %s", profileName, slice.Name)
	}
	placements := slice.Status.NodeResources.MigPlacement[profileName].Placements
	for _, placement := range placements {
		if isPlacementAvailable(placement, slice) {
			resourceID := types.UID(fmt.Sprintf("%s-%s-%d", pod.Name, slice.Name, placement.Start))
			allocReq, allocRes := policy.SetAllocationDetails(
				profileName, placement.Start, size, pod.UID, types.NodeName(slice.Name),
				inferencev1alpha1.AllocationStatus{AllocationStatusController: string(inferencev1alpha1.AllocationStatusCreating)},
				giProfile, ciProfileID, ciEngProfileID,
				pod.Namespace, pod.Name, slice.Status.NodeResources.NodeGPUs[0].GPUUUID, resourceID,
			)
			return allocReq, allocRes, nil
		}
	}
	return nil, nil, fmt.Errorf("no available placement found for profile %s on node %s", profileName, slice.Name)
}

// extractGpuProfile retrieves profile size and IDs from Instaslice status
func extractGpuProfile(slice *inferencev1alpha1.Instaslice, profileName string) (int32, int32, int32, int32) {
	var size, giProfile, ciProfileID, ciEngProfileID int32
	if placementStatus, exists := slice.Status.NodeResources.MigPlacement[profileName]; exists {
		for _, p := range placementStatus.Placements {
			size = p.Size
			giProfile = placementStatus.GIProfileID
			ciProfileID = placementStatus.CIProfileID
			ciEngProfileID = placementStatus.CIEngProfileID
			break
		}
	}
	return size, giProfile, ciProfileID, ciEngProfileID
}

// isPlacementAvailable checks if the given placement is not already allocated
func isPlacementAvailable(placement inferencev1alpha1.Placement, slice *inferencev1alpha1.Instaslice) bool {
	for _, alloc := range slice.Status.PodAllocationResults {
		if alloc.MigPlacement.Start == placement.Start && alloc.MigPlacement.Size == placement.Size {
			return false
		}
	}
	return true
}

func (c *PodController) nameToKey(obj runtime.Object) []string {
	metaObj, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		klog.Errorf("the object is not a metav1.ObjectMetaAccessor: %T", obj)
		return []string{}
	}
	meta := metaObj.GetObjectMeta()
	if meta.GetNamespace() == "" {
		return []string{meta.GetName()}
	}
	return []string{meta.GetNamespace() + "/" + meta.GetName()}
}
