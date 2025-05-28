package instaslice

import (
	"context"
	"fmt"

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

type InstasliceController struct {
	namespace          string
	instasliceClient   *clientset.Clientset
	kubeClient         *kubernetes.Clientset
	instasliceInformer cache.SharedIndexInformer
	eventRecorder      events.Recorder
	allocationCache    map[types.UID]inferencev1alpha1.AllocationResult
	isCacheInitialized bool
}

type InstasliceControllerConfig struct {
	Namespace          string
	OperatorClient     *clientset.Clientset
	KubeClient         *kubernetes.Clientset
	InstasliceInformer cache.SharedIndexInformer
	EventRecorder      events.Recorder
}

func NewInstasliceController(config *InstasliceControllerConfig) factory.Controller {
	c := &InstasliceController{
		namespace:          config.Namespace,
		instasliceClient:   config.OperatorClient,
		kubeClient:         config.KubeClient,
		instasliceInformer: config.InstasliceInformer,
		eventRecorder:      config.EventRecorder,
		allocationCache:    make(map[types.UID]inferencev1alpha1.AllocationResult),
		isCacheInitialized: false,
	}

	return factory.New().
		WithSync(c.sync).
		WithInformersQueueKeysFunc(c.nameToKey, c.instasliceInformer).
		ToController("InstasliceController", c.eventRecorder)
}

func (c *InstasliceController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	instasliceName := syncCtx.QueueKey()

	klog.V(2).InfoS("Instaslice Sync", "queue_key", instasliceName)

	slice, err := c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(c.namespace).Get(ctx, instasliceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Rebuild cache on node failure
	node, err := c.kubeClient.CoreV1().Nodes().Get(ctx, slice.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to get Node", "node", slice.Name)
			return err
		}
	} else {
		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady && condition.Status != v1.ConditionTrue {
				klog.InfoS("Detected a node going down", "node", node.Name)
				if err := c.rebuildAllocationCache(ctx); err != nil {
					return err
				}
				break
			}
		}
	}

	// Validate boot ID synchronization only if node exists
	if node != nil && slice.Status.NodeResources.BootID != "" && slice.Status.NodeResources.BootID != node.Status.NodeInfo.BootID {
		err := fmt.Errorf("instaslice not in sync with the node as the boot id doesn't match")
		klog.ErrorS(err, "instaslice's boot id not matching node's boot id",
			"node_boot_id", node.Status.NodeInfo.BootID,
			"instaslice_boot_id", slice.Status.NodeResources.BootID)
		return fmt.Errorf("boot ID mismatch, requeueing in %v", constants.Requeue1sDelay)
	}

	// Process allocation logic
	return c.processAllocations(ctx, slice)
}

// queueKeysRuntimeForObj is an adapter on top of queueKeysForObj to be used in
// factory.Controller queueing functions
func (c *InstasliceController) nameToKey(obj runtime.Object) []string {
	metaObj, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		klog.Errorf("the object is not a metav1.ObjectMetaAccessor: %T", obj)
		return []string{}
	}
	return []string{metaObj.GetObjectMeta().GetName()}
}

// processAllocations handles the core allocation logic for GPU slices
func (c *InstasliceController) processAllocations(ctx context.Context, slice *inferencev1alpha1.Instaslice) error {
	klog.V(2).InfoS("Processing allocations", "slice", slice.Name)

	// Clean up orphaned allocations
	c.cleanupOrphanedAllocations(ctx, slice)

	// Process each allocation result
	for podUIDStr, allocResult := range slice.Status.PodAllocationResults {
		podUID := types.UID(podUIDStr)
		if err := c.processAllocationResult(ctx, slice, podUID, allocResult); err != nil {
			return err
		}
	}

	return nil
}

// processAllocationResult processes a single allocation result
func (c *InstasliceController) processAllocationResult(ctx context.Context, slice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult inferencev1alpha1.AllocationResult) error {

	allocRequest, exists := (*slice.Spec.PodAllocationRequests)[podUID]
	if !exists {
		klog.V(2).InfoS("No allocation request found for result", "podUID", podUID)
		return nil
	}

	// Handle different allocation states
	switch {
	case allocResult.AllocationStatus.AllocationStatusDaemonset == string(inferencev1alpha1.AllocationStatusCreated) &&
		allocResult.AllocationStatus.AllocationStatusController != string(inferencev1alpha1.AllocationStatusUngated):
		return c.handleCreatedAllocation(ctx, slice, podUID, &allocResult, &allocRequest)
	case allocResult.AllocationStatus.AllocationStatusController == string(inferencev1alpha1.AllocationStatusUngated):
		return c.handleUngatedAllocation(ctx, slice, podUID, &allocResult, &allocRequest)
	case allocResult.AllocationStatus.AllocationStatusController == string(inferencev1alpha1.AllocationStatusDeleting):
		return c.handleDeletingAllocation(ctx, slice, podUID, &allocResult, &allocRequest)
	}

	return nil
}

// handleCreatedAllocation processes allocations that are created by daemonset
func (c *InstasliceController) handleCreatedAllocation(ctx context.Context, slice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult *inferencev1alpha1.AllocationResult, allocRequest *inferencev1alpha1.AllocationRequest) error {

	klog.V(2).InfoS("Handling created allocation", "pod", allocRequest.PodRef.Name, "node", allocResult.Nodename)

	// Update allocation status to ungated
	allocResult.AllocationStatus.AllocationStatusController = string(inferencev1alpha1.AllocationStatusUngated)

	return utils.UpdateInstasliceAllocationWithClientset(ctx, c.instasliceClient, slice.Name, c.namespace, allocResult, allocRequest)
}

// handleUngatedAllocation processes allocations that need pod ungating
func (c *InstasliceController) handleUngatedAllocation(ctx context.Context, slice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult *inferencev1alpha1.AllocationResult, allocRequest *inferencev1alpha1.AllocationRequest) error {

	klog.V(2).InfoS("Handling ungated allocation", "pod", allocRequest.PodRef.Name, "node", allocResult.Nodename)

	// Get the pod and ungate it
	pod, err := c.kubeClient.CoreV1().Pods(allocRequest.PodRef.Namespace).Get(ctx, allocRequest.PodRef.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Pod no longer exists, clean up allocation
			return c.cleanupAllocation(ctx, slice, podUID, allocResult, allocRequest)
		}
		return err
	}

	return c.addNodeSelectorAndUngatePod(ctx, pod, allocResult)
}

// handleDeletingAllocation processes allocations that are being deleted
func (c *InstasliceController) handleDeletingAllocation(ctx context.Context, slice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult *inferencev1alpha1.AllocationResult, allocRequest *inferencev1alpha1.AllocationRequest) error {

	klog.V(2).InfoS("Handling deleting allocation", "pod", allocRequest.PodRef.Name, "node", allocResult.Nodename)

	// Wait for daemonset to complete deletion
	if allocResult.AllocationStatus.AllocationStatusDaemonset == string(inferencev1alpha1.AllocationStatusDeleted) {
		return c.removeInstasliceAllocation(ctx, slice.Name, allocResult)
	}

	return nil
}

// addNodeSelectorAndUngatePod adds node selector and removes scheduling gate from pod
func (c *InstasliceController) addNodeSelectorAndUngatePod(ctx context.Context, pod *v1.Pod, allocResult *inferencev1alpha1.AllocationResult) error {
	// Add node selector
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = make(map[string]string)
	}
	pod.Spec.NodeSelector[constants.NodeLabel] = string(allocResult.Nodename)

	// Remove scheduling gate
	ungatedPod := utils.UnGatePod(pod)

	_, err := c.kubeClient.CoreV1().Pods(pod.Namespace).Update(ctx, ungatedPod, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "error ungating pod", "pod", pod.Name)
		return err
	}

	klog.InfoS("Pod ungated successfully", "pod", pod.Name, "node", allocResult.Nodename)
	return nil
}

// cleanupAllocation removes an allocation from the Instaslice object
func (c *InstasliceController) cleanupAllocation(ctx context.Context, slice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult *inferencev1alpha1.AllocationResult, allocRequest *inferencev1alpha1.AllocationRequest) error {

	klog.V(2).InfoS("Cleaning up allocation", "pod", allocRequest.PodRef.Name)

	// Remove from both spec and status
	delete(*slice.Spec.PodAllocationRequests, podUID)
	delete(slice.Status.PodAllocationResults, string(podUID))

	// Update the object
	_, err := c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(c.namespace).Update(ctx, slice, metav1.UpdateOptions{})
	return err
}

// removeInstasliceAllocation removes allocation after daemonset cleanup
func (c *InstasliceController) removeInstasliceAllocation(ctx context.Context, instasliceName string, allocResult *inferencev1alpha1.AllocationResult) error {
	if allocResult.AllocationStatus.AllocationStatusDaemonset == string(inferencev1alpha1.AllocationStatusDeleted) {
		return utils.UpdateInstasliceAllocationWithClientset(ctx, c.instasliceClient, instasliceName, c.namespace, nil, nil)
	}
	return nil
}

// rebuildAllocationCache rebuilds the internal allocation cache
func (c *InstasliceController) rebuildAllocationCache(ctx context.Context) error {
	klog.V(2).InfoS("Rebuilding allocation cache")

	instasliceList, err := c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	c.allocationCache = make(map[types.UID]inferencev1alpha1.AllocationResult)

	for _, instaslice := range instasliceList.Items {
		for podUIDStr, allocation := range instaslice.Status.PodAllocationResults {
			c.allocationCache[types.UID(podUIDStr)] = allocation
		}
	}

	c.isCacheInitialized = true
	klog.V(2).InfoS("Allocation cache rebuilt", "entries", len(c.allocationCache))
	return nil
}

// cleanupOrphanedAllocations removes allocations for non-existent pods
func (c *InstasliceController) cleanupOrphanedAllocations(ctx context.Context, slice *inferencev1alpha1.Instaslice) {
	if slice.Spec.PodAllocationRequests == nil {
		return
	}

	for podUID, allocRequest := range *slice.Spec.PodAllocationRequests {
		// Check if pod still exists
		_, err := c.kubeClient.CoreV1().Pods(allocRequest.PodRef.Namespace).Get(ctx, allocRequest.PodRef.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			klog.V(2).InfoS("Found orphaned allocation, cleaning up", "pod", allocRequest.PodRef.Name)

			// Remove from maps
			delete(*slice.Spec.PodAllocationRequests, podUID)
			delete(slice.Status.PodAllocationResults, string(podUID))
		}
	}
}

// findNodeAndDeviceForSlice implements allocation policy to find suitable GPU placement
func (c *InstasliceController) findNodeAndDeviceForSlice(ctx context.Context, slice *inferencev1alpha1.Instaslice,
	profileName string, policy utils.AllocationPolicy, pod *v1.Pod) (*inferencev1alpha1.AllocationRequest, *inferencev1alpha1.AllocationResult, error) {

	klog.V(2).InfoS("Finding node and device for slice", "profile", profileName, "pod", pod.Name)

	// Extract GPU profile information
	size, discoveredGiprofile, ciProfileID, ciEngProfileID := c.extractGpuProfile(slice, profileName)
	if size == 0 {
		return nil, nil, fmt.Errorf("profile %s not found on node %s", profileName, slice.Name)
	}

	// Find available placement
	for _, placement := range slice.Status.NodeResources.MigPlacement[profileName].Placements {
		if c.isPlacementAvailable(placement, slice) {
			// Generate unique resource identifier
			resourceIdentifier := types.UID(fmt.Sprintf("%s-%s-%d", pod.Name, slice.Name, placement.Start))

			allocRequest, allocResult := policy.SetAllocationDetails(
				profileName, placement.Start, size, pod.UID, types.NodeName(slice.Name),
				inferencev1alpha1.AllocationStatus{
					AllocationStatusController: string(inferencev1alpha1.AllocationStatusCreating),
				},
				discoveredGiprofile, ciProfileID, ciEngProfileID,
				pod.Namespace, pod.Name, slice.Status.NodeResources.NodeGPUs[0].GPUUUID, resourceIdentifier,
			)

			return allocRequest, allocResult, nil
		}
	}

	return nil, nil, fmt.Errorf("no available placement found for profile %s on node %s", profileName, slice.Name)
}

// extractGpuProfile extracts GPU profile information for allocation
func (c *InstasliceController) extractGpuProfile(slice *inferencev1alpha1.Instaslice, profileName string) (int32, int32, int32, int32) {
	var size int32
	var discoveredGiprofile int32
	var ciProfileID int32
	var ciEngProfileID int32

	if placement, exists := slice.Status.NodeResources.MigPlacement[profileName]; exists {
		for _, aPlacement := range placement.Placements {
			size = aPlacement.Size
			discoveredGiprofile = placement.GIProfileID
			ciProfileID = placement.CIProfileID
			ciEngProfileID = placement.CIEngProfileID
			break
		}
	}

	return size, discoveredGiprofile, ciProfileID, ciEngProfileID
}

// isPlacementAvailable checks if a GPU placement is available for allocation
func (c *InstasliceController) isPlacementAvailable(placement inferencev1alpha1.Placement, slice *inferencev1alpha1.Instaslice) bool {
	// Check if this placement is already allocated
	for _, allocResult := range slice.Status.PodAllocationResults {
		if allocResult.MigPlacement.Start == placement.Start && allocResult.MigPlacement.Size == placement.Size {
			// This placement is already taken
			return false
		}
	}
	return true
}
