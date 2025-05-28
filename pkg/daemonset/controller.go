package daemonset

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
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

type DaemonsetController struct {
	kubeClient         *kubernetes.Clientset
	instasliceClient   *clientset.Clientset
	instasliceInformer cache.SharedInformer
	nodeInformer       cache.SharedInformer
	eventRecorder      events.Recorder
	nodeName           string
	emulatorMode       bool
}

type DaemonsetControllerConfig struct {
	KubeClient         *kubernetes.Clientset
	InstasliceClient   *clientset.Clientset
	InstasliceInformer cache.SharedInformer
	NodeInformer       cache.SharedInformer
	EventRecorder      events.Recorder
	NodeName           string
	EmulatorMode       bool
}

// MigProfile represents a MIG profile in human-readable format
type MigProfile struct {
	C              int
	G              int
	GB             int
	GIProfileID    int
	CIProfileID    int
	CIEngProfileID int
}

// MigDeviceInfo holds MIG device references discovered via NVML
type MigDeviceInfo struct {
	uuid   string
	giInfo *nvml.GpuInstanceInfo
	ciInfo *nvml.ComputeInstanceInfo
	start  int32
	size   int32
}

// ResPatchOperation for JSON patch operations on node resources
type ResPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

var (
	discoveredGpusOnHost []string
	initNvmlOnce         sync.Once
)

func NewDaemonsetController(config *DaemonsetControllerConfig) factory.Controller {
	c := &DaemonsetController{
		kubeClient:         config.KubeClient,
		instasliceClient:   config.InstasliceClient,
		instasliceInformer: config.InstasliceInformer,
		nodeInformer:       config.NodeInformer,
		eventRecorder:      config.EventRecorder,
		nodeName:           config.NodeName,
		emulatorMode:       config.EmulatorMode,
	}

	// Initialize NVML if not in emulator mode
	var err error
	initNvmlOnce.Do(func() {
		if !config.EmulatorMode {
			ret := nvml.Init()
			if ret != nvml.SUCCESS {
				err = fmt.Errorf("unable to initialize NVML: %v", ret)
			}
		}
	})

	if err != nil {
		klog.ErrorS(err, "Failed to initialize NVML")
	}

	return factory.New().
		WithSync(c.sync).
		WithInformersQueueKeysFunc(c.nameToKey, c.instasliceInformer, c.nodeInformer).
		ToController("DaemonsetController", c.eventRecorder)
}

func (c *DaemonsetController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	queueKey := syncCtx.QueueKey()
	klog.V(2).InfoS("Daemonset Sync", "queue_key", queueKey)

	// Only process events for this node
	if queueKey != c.nodeName {
		return nil
	}

	nsName := types.NamespacedName{
		Name:      c.nodeName,
		Namespace: constants.InstaSliceOperatorNamespace,
	}

	instaslice, err := c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(nsName.Namespace).Get(ctx, nsName.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		klog.ErrorS(err, "Error getting Instaslice", "name", c.nodeName)
		return factory.SyntheticRequeueError
	}

	// Get the node object
	node, err := c.kubeClient.CoreV1().Nodes().Get(ctx, c.nodeName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "error getting the node object", "name", c.nodeName)
		return factory.SyntheticRequeueError
	}

	// Update the instaslice object with the Node object's current BootID
	if err := c.syncBootID(ctx, instaslice, node); err != nil {
		return err
	}

	// Process allocations
	return c.processAllocations(ctx, instaslice)
}

func (c *DaemonsetController) syncBootID(ctx context.Context, instaslice *inferencev1alpha1.Instaslice, node *v1.Node) error {
	if instaslice.Status.NodeResources.BootID == "" {
		// Set boot ID for the first time
		instaslice.Status.NodeResources.BootID = node.Status.NodeInfo.BootID
		_, err := c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(constants.InstaSliceOperatorNamespace).
			UpdateStatus(ctx, instaslice, metav1.UpdateOptions{})
		if err != nil {
			klog.ErrorS(err, "error patching instaslice object with Boot ID", "nodeName", c.nodeName, "bootId", node.Status.NodeInfo.BootID)
			return factory.SyntheticRequeueError
		}
	} else if instaslice.Status.NodeResources.BootID != node.Status.NodeInfo.BootID {
		// Boot ID changed - need to recreate all allocations
		for podUID := range instaslice.Status.PodAllocationResults {
			if err := c.createCiAndGiProfiles(ctx, instaslice, types.UID(podUID)); err != nil {
				return factory.SyntheticRequeueError
			}
		}

		// Update boot ID
		instaslice.Status.NodeResources.BootID = node.Status.NodeInfo.BootID
		_, err := c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(constants.InstaSliceOperatorNamespace).
			UpdateStatus(ctx, instaslice, metav1.UpdateOptions{})
		if err != nil {
			klog.ErrorS(err, "error patching instaslice object with Boot ID", "nodeName", c.nodeName, "bootId", node.Status.NodeInfo.BootID)
			return factory.SyntheticRequeueError
		}
	}

	return nil
}

func (c *DaemonsetController) processAllocations(ctx context.Context, instaslice *inferencev1alpha1.Instaslice) error {
	for podUID, allocResult := range instaslice.Status.PodAllocationResults {
		// Only process allocations for this node
		if allocResult.Nodename != types.NodeName(c.nodeName) {
			continue
		}

		podRef := (*instaslice.Spec.PodAllocationRequests)[types.UID(podUID)].PodRef

		// Handle deleting allocations
		if inferencev1alpha1.AllocationStatusController(allocResult.AllocationStatus.AllocationStatusController) == inferencev1alpha1.AllocationStatusDeleting &&
			inferencev1alpha1.AllocationStatusDaemonset(allocResult.AllocationStatus.AllocationStatusDaemonset) == inferencev1alpha1.AllocationStatusCreated {

			klog.InfoS("Performing cleanup for pod", "podRef", podRef.Name)
			if err := c.handleDeletingAllocation(ctx, instaslice, types.UID(podUID), &allocResult, &podRef); err != nil {
				return err
			}
			return nil
		}

		// Handle creating allocations
		if inferencev1alpha1.AllocationStatusController(allocResult.AllocationStatus.AllocationStatusController) == inferencev1alpha1.AllocationStatusCreating &&
			allocResult.AllocationStatus.AllocationStatusDaemonset == "" {

			klog.InfoS("Creating allocation for pod", "podRef", podRef.Name)
			if err := c.handleCreatingAllocation(ctx, instaslice, types.UID(podUID), &allocResult, &podRef); err != nil {
				return err
			}
			return nil
		}
	}

	return nil
}

func (c *DaemonsetController) handleDeletingAllocation(ctx context.Context, instaslice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult *inferencev1alpha1.AllocationResult, podRef *v1.ObjectReference) error {

	if !c.emulatorMode {
		exists, err := c.checkConfigMapExists(ctx, string(allocResult.ConfigMapResourceIdentifier), podRef.Namespace)
		if err != nil {
			klog.ErrorS(err, "error checking configmap existence", "podRef", podRef.Name)
			return factory.SyntheticRequeueError
		}
		if exists {
			if err := c.cleanUpCiAndGi(ctx, allocResult, *podRef); err != nil {
				klog.ErrorS(err, "error cleaning up ci and gi retrying", "podRef", podRef.Name)
				return factory.SyntheticRequeueError
			}
		}
	}

	err := c.deleteConfigMap(ctx, string(allocResult.ConfigMapResourceIdentifier), podRef.Namespace)
	if err != nil && !errors.IsNotFound(err) {
		klog.ErrorS(err, "error deleting config map for pod", "pod", podRef.Name)
		return err
	}

	// Update allocation status to deleted
	newAlloc := *allocResult
	newAlloc.AllocationStatus.AllocationStatusDaemonset = string(inferencev1alpha1.AllocationStatusDeleted)
	instaslice.Status.PodAllocationResults[string(podUID)] = newAlloc

	_, err = c.instasliceClient.OpenShiftOperatorV1alpha1().Instaslices(constants.InstaSliceOperatorNamespace).
		UpdateStatus(ctx, instaslice, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "error updating Instaslice status for pod cleanup", "podRef", podRef.Name)
		return err
	}

	return nil
}

func (c *DaemonsetController) handleCreatingAllocation(ctx context.Context, instaslice *inferencev1alpha1.Instaslice,
	podUID types.UID, allocResult *inferencev1alpha1.AllocationResult, podRef *v1.ObjectReference) error {

	exists, err := c.checkConfigMapExists(ctx, string(allocResult.ConfigMapResourceIdentifier), podRef.Namespace)
	if err != nil {
		klog.ErrorS(err, "error obtaining configmap", string(allocResult.ConfigMapResourceIdentifier))
		return factory.SyntheticRequeueError
	}

	allocationRequest, haveReq := (*instaslice.Spec.PodAllocationRequests)[podUID]
	if !haveReq {
		klog.InfoS("No matching PodAllocationRequest for this result; skipping", "podRef", podRef.Name)
		return nil
	}

	if !exists {
		if err := c.createMIGSlice(ctx, instaslice, &allocationRequest, allocResult); err != nil {
			klog.ErrorS(err, "MIG slice creation failed", "podRef", podRef.Name)
			return factory.SyntheticRequeueError
		}
	}

	// Update allocation status to created
	newAllocationResult := *allocResult
	newAllocationResult.AllocationStatus.AllocationStatusDaemonset = string(inferencev1alpha1.AllocationStatusCreated)

	return utils.UpdateInstasliceAllocationWithClientset(ctx, c.instasliceClient, instaslice.Name,
		constants.InstaSliceOperatorNamespace, &newAllocationResult, &allocationRequest)
}

func (c *DaemonsetController) createMIGSlice(ctx context.Context, instaslice *inferencev1alpha1.Instaslice,
	allocRequest *inferencev1alpha1.AllocationRequest, allocResult *inferencev1alpha1.AllocationResult) error {

	if c.emulatorMode {
		// Emulated mode - create fake configmap
		err := c.createConfigMap(ctx,
			string(allocResult.ConfigMapResourceIdentifier),
			allocRequest.PodRef.Namespace,
			string(allocResult.ConfigMapResourceIdentifier))
		if err != nil {
			return fmt.Errorf("failed to create config map (emulator mode): %w", err)
		}
		// Simulate MIG creation delay
		time.Sleep(constants.Requeue1sDelay)
		return nil
	}

	// Real mode - create actual MIG slices
	device, retCode := nvml.DeviceGetHandleByUUID(allocResult.GPUUUID)
	if retCode != nvml.SUCCESS {
		return fmt.Errorf("error getting GPU device handle %s: %v", allocResult.GPUUUID, retCode)
	}

	selectedMig, ok := instaslice.Status.NodeResources.MigPlacement[allocRequest.Profile]
	if !ok {
		return fmt.Errorf("no suitable MIG profile in NodeResources for %s", allocRequest.Profile)
	}

	placement := nvml.GpuInstancePlacement{
		Start: uint32(allocResult.MigPlacement.Start),
		Size:  uint32(allocResult.MigPlacement.Size),
	}

	giProfileInfo, retGI := device.GetGpuInstanceProfileInfo(int(selectedMig.GIProfileID))
	if retGI != nvml.SUCCESS {
		return fmt.Errorf("error getting GPU instance profile info %d: %v", selectedMig.GIProfileID, retGI)
	}

	createdMigInfos, err := c.createSliceAndPopulateMigInfos(
		ctx, device, giProfileInfo, placement, selectedMig.CIProfileID, allocRequest.PodRef.Name)
	if err != nil {
		return fmt.Errorf("MIG creation not successful: %w", err)
	}

	// Find the created MIG device and create configmap
	for migUuid, migDevice := range createdMigInfos {
		if migDevice.start == allocResult.MigPlacement.Start &&
			migDevice.uuid == allocResult.GPUUUID &&
			giProfileInfo.Id == migDevice.giInfo.ProfileId {

			if err := c.createConfigMap(ctx, migUuid, allocRequest.PodRef.Namespace,
				string(allocResult.ConfigMapResourceIdentifier)); err != nil {
				return err
			}
			klog.InfoS("done creating mig slice", "pod", allocRequest.PodRef.Name,
				"parentgpu", allocResult.GPUUUID, "miguuid", migUuid)
			break
		}
	}

	return nil
}

func (c *DaemonsetController) nameToKey(obj runtime.Object) []string {
	metaObj, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		klog.Errorf("the object is not a metav1.ObjectMetaAccessor: %T", obj)
		return []string{}
	}
	return []string{metaObj.GetObjectMeta().GetName()}
}
