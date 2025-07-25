package metrics

import (
	"time"

	slicev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/dasoperator/v1alpha1"
	"k8s.io/klog/v2"
)

// AllocationClaimTracker helps track AllocationClaim lifecycle metrics
type AllocationClaimTracker struct {
	manager *MetricsManager
}

// NewAllocationClaimTracker creates a new tracker for AllocationClaim metrics
func NewAllocationClaimTracker() *AllocationClaimTracker {
	return &AllocationClaimTracker{
		manager: NewMetricsManager(),
	}
}

// TrackAllocationClaimCreation starts tracking a new AllocationClaim
func (t *AllocationClaimTracker) TrackAllocationClaimCreation(claimName, namespace string) {
	t.manager.StartAllocationClaimTimer(claimName, namespace)
	klog.V(4).InfoS("Started tracking AllocationClaim", "claim", claimName, "namespace", namespace)
}

// TrackAllocationClaimStateTransition records a state transition
func (t *AllocationClaimTracker) TrackAllocationClaimStateTransition(fromState, toState slicev1alpha1.AllocationClaimState, namespace string) {
	t.manager.RecordAllocationClaimTransition(fromState, toState, namespace)
	klog.V(4).InfoS("AllocationClaim state transition", "from", fromState, "to", toState, "namespace", namespace)
}

// TrackAllocationClaimCompletion records the completion of an AllocationClaim
func (t *AllocationClaimTracker) TrackAllocationClaimCompletion(claimName, namespace string, finalState slicev1alpha1.AllocationClaimState) {
	t.manager.EndAllocationClaimTimer(claimName, namespace, finalState)
	klog.V(4).InfoS("Completed tracking AllocationClaim", "claim", claimName, "namespace", namespace, "final_state", finalState)
}

// GPUInventoryTracker helps track GPU inventory metrics
type GPUInventoryTracker struct {
	manager *MetricsManager
}

// NewGPUInventoryTracker creates a new tracker for GPU inventory metrics
func NewGPUInventoryTracker() *GPUInventoryTracker {
	return &GPUInventoryTracker{
		manager: NewMetricsManager(),
	}
}

// UpdateGPUInventory updates metrics for a specific GPU and profile
func (t *GPUInventoryTracker) UpdateGPUInventory(node, gpuUUID, profile string, capacity, allocated int32) {
	t.manager.RecordGPUInventory(node, gpuUUID, profile, capacity, allocated)
	klog.V(4).InfoS("Updated GPU inventory metrics", "node", node, "gpu_uuid", gpuUUID, "profile", profile, "capacity", capacity, "allocated", allocated)
}

// UpdateAllocationClaimState updates the state count for AllocationClaims
func (t *GPUInventoryTracker) UpdateAllocationClaimState(state slicev1alpha1.AllocationClaimState, namespace string) {
	t.manager.RecordAllocationClaimState(state, namespace)
	klog.V(4).InfoS("Updated AllocationClaim state metrics", "state", state, "namespace", namespace)
}

// SchedulerMetricsTracker helps track scheduler-related metrics
type SchedulerMetricsTracker struct {
	manager *MetricsManager
}

// NewSchedulerMetricsTracker creates a new tracker for scheduler metrics
func NewSchedulerMetricsTracker() *SchedulerMetricsTracker {
	return &SchedulerMetricsTracker{
		manager: NewMetricsManager(),
	}
}

// TrackFilterResult records the result of a scheduler filter operation
func (t *SchedulerMetricsTracker) TrackFilterResult(result string, node string) {
	t.manager.RecordSchedulerFilterResult(result, node)
	klog.V(4).InfoS("Recorded scheduler filter result", "result", result, "node", node)
}

// TrackScoreInvocation records a scheduler score invocation
func (t *SchedulerMetricsTracker) TrackScoreInvocation(node string) {
	t.manager.RecordSchedulerScore(node)
	klog.V(4).InfoS("Recorded scheduler score invocation", "node", node)
}

// TrackPrebindLatency records the latency of a prebind operation
func (t *SchedulerMetricsTracker) TrackPrebindLatency(node string, duration time.Duration) {
	t.manager.RecordSchedulerPrebindLatency(node, duration)
	klog.V(4).InfoS("Recorded scheduler prebind latency", "node", node, "duration", duration)
}

// DevicePluginMetricsTracker helps track device plugin metrics
type DevicePluginMetricsTracker struct {
	manager *MetricsManager
}

// NewDevicePluginMetricsTracker creates a new tracker for device plugin metrics
func NewDevicePluginMetricsTracker() *DevicePluginMetricsTracker {
	return &DevicePluginMetricsTracker{
		manager: NewMetricsManager(),
	}
}

// TrackSliceProvision records slice provisioning results
func (t *DevicePluginMetricsTracker) TrackSliceProvision(result, profile, node string, duration time.Duration) {
	t.manager.RecordSliceProvision(result, profile, node, duration)
	klog.V(4).InfoS("Recorded slice provision", "result", result, "profile", profile, "node", node, "duration", duration)
}

// TrackSliceDeletion records slice deletion results
func (t *DevicePluginMetricsTracker) TrackSliceDeletion(result, profile, node string, duration time.Duration) {
	t.manager.RecordSliceDeletion(result, profile, node, duration)
	klog.V(4).InfoS("Recorded slice deletion", "result", result, "profile", profile, "node", node, "duration", duration)
}

// ControllerMetricsTracker helps track controller reconciliation metrics
type ControllerMetricsTracker struct {
	manager *MetricsManager
}

// NewControllerMetricsTracker creates a new tracker for controller metrics
func NewControllerMetricsTracker() *ControllerMetricsTracker {
	return &ControllerMetricsTracker{
		manager: NewMetricsManager(),
	}
}

// TrackReconcileResult records reconciliation results
func (t *ControllerMetricsTracker) TrackReconcileResult(controller, result string, duration time.Duration) {
	t.manager.RecordReconcileResult(controller, result, duration)
	klog.V(4).InfoS("Recorded reconcile result", "controller", controller, "result", result, "duration", duration)
}

// TrackError records errors from components
func (t *ControllerMetricsTracker) TrackError(component, errorType string) {
	t.manager.RecordError(component, errorType)
	klog.V(4).InfoS("Recorded error", "component", component, "error_type", errorType)
}

// Global metrics tracker instances for easy access
var (
	GlobalAllocationClaimTracker = NewAllocationClaimTracker()
	GlobalGPUInventoryTracker    = NewGPUInventoryTracker()
	GlobalSchedulerTracker       = NewSchedulerMetricsTracker()
	GlobalDevicePluginTracker    = NewDevicePluginMetricsTracker()
	GlobalControllerTracker      = NewControllerMetricsTracker()
)
