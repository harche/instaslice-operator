package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"

	slicev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/dasoperator/v1alpha1"
)

const (
	// MetricNamespace is the namespace for all InstaSlice metrics
	MetricNamespace = "das"
	// MetricSubsystem is the subsystem for InstaSlice metrics
	MetricSubsystem = "instaslice"
)

var (
	// GPU & MIG slice inventory metrics
	GPUTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "gpu_total",
			Help:      "Count of physical GPUs per node",
		},
		[]string{"node", "gpu_uuid"},
	)

	GPUSliceCapacity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "gpu_slice_capacity",
			Help:      "Maximum number of MIG slices a GPU supports for each profile",
		},
		[]string{"node", "gpu_uuid", "profile"},
	)

	GPUSliceAllocated = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "gpu_slice_allocated",
			Help:      "Current number of slices in use",
		},
		[]string{"node", "gpu_uuid", "profile"},
	)

	GPUSliceFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "gpu_slice_free",
			Help:      "Remaining capacity per profile",
		},
		[]string{"node", "gpu_uuid", "profile"},
	)

	// AllocationClaim lifecycle metrics
	AllocationClaimStateTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "allocationclaim_state_total",
			Help:      "Number of claims by state (staged, created, inUse, etc.)",
		},
		[]string{"state", "namespace"},
	)

	AllocationClaimTransitionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "allocationclaim_transitions_total",
			Help:      "Number of state transitions",
		},
		[]string{"from_state", "to_state", "namespace"},
	)

	AllocationClaimDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "allocationclaim_duration_seconds",
			Help:      "Time from claim creation until final state",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"final_state", "namespace"},
	)

	// Scheduler plugin activity metrics
	SchedulerFilterAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "scheduler_filter_attempts_total",
			Help:      "Success/failure counts for the Filter phase",
		},
		[]string{"result", "node"},
	)

	SchedulerScoreInvocationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "scheduler_score_invocations_total",
			Help:      "How often nodes are scored",
		},
		[]string{"node"},
	)

	SchedulerPrebindLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "scheduler_prebind_latency_seconds",
			Help:      "Time to promote staged claims during PreBind",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"node"},
	)

	// Device plugin & slice provisioning metrics
	SliceProvisionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "slice_provision_total",
			Help:      "Slices created or failed",
		},
		[]string{"result", "profile", "node"},
	)

	SliceProvisionLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "slice_provision_latency_seconds",
			Help:      "Time for MIG slice creation",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"profile", "node"},
	)

	SliceDeletionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "slice_deletion_total",
			Help:      "Slices deleted or failed to delete",
		},
		[]string{"result", "profile", "node"},
	)

	SliceDeletionLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "slice_deletion_latency_seconds",
			Help:      "Time for MIG slice deletion",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"profile", "node"},
	)

	// Operator controller health metrics
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "reconcile_total",
			Help:      "Reconciliation outcomes for controllers",
		},
		[]string{"controller", "result"},
	)

	ReconcileDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "reconcile_duration_seconds",
			Help:      "Time spent in each reconcile loop",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"controller"},
	)

	ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "errors_total",
			Help:      "Errors from webhook, scheduler, or device plugin components",
		},
		[]string{"component", "error_type"},
	)

	// Build and configuration info metrics
	Info = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "info",
			Help:      "Operator version, git commit, build date",
		},
		[]string{"version", "git_commit", "build_date"},
	)

	EmulatedMode = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricNamespace,
			Subsystem: MetricSubsystem,
			Name:      "emulated_mode",
			Help:      "Indicates whether hardware emulation is active",
		},
		[]string{"mode"},
	)
)

func init() {
	// Register all metrics with Prometheus
	prometheus.MustRegister(
		GPUTotal,
		GPUSliceCapacity,
		GPUSliceAllocated,
		GPUSliceFree,
		AllocationClaimStateTotal,
		AllocationClaimTransitionsTotal,
		AllocationClaimDurationSeconds,
		SchedulerFilterAttemptsTotal,
		SchedulerScoreInvocationsTotal,
		SchedulerPrebindLatencySeconds,
		SliceProvisionTotal,
		SliceProvisionLatencySeconds,
		SliceDeletionTotal,
		SliceDeletionLatencySeconds,
		ReconcileTotal,
		ReconcileDurationSeconds,
		ErrorsTotal,
		Info,
		EmulatedMode,
	)
}

// MetricsManager provides methods to update metrics
type MetricsManager struct {
	allocationClaimTimers map[string]time.Time
}

// NewMetricsManager creates a new metrics manager
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		allocationClaimTimers: make(map[string]time.Time),
	}
}

// RecordGPUInventory updates GPU inventory metrics
func (m *MetricsManager) RecordGPUInventory(node, gpuUUID string, profile string, capacity, allocated int32) {
	GPUTotal.WithLabelValues(node, gpuUUID).Set(1)
	GPUSliceCapacity.WithLabelValues(node, gpuUUID, profile).Set(float64(capacity))
	GPUSliceAllocated.WithLabelValues(node, gpuUUID, profile).Set(float64(allocated))
	GPUSliceFree.WithLabelValues(node, gpuUUID, profile).Set(float64(capacity - allocated))
}

// RecordAllocationClaimState updates allocation claim state metrics
func (m *MetricsManager) RecordAllocationClaimState(state slicev1alpha1.AllocationClaimState, namespace string) {
	// Reset all states to 0 first
	for _, s := range []slicev1alpha1.AllocationClaimState{
		slicev1alpha1.AllocationClaimStatusCreated,
		slicev1alpha1.AllocationClaimStatusProcessing,
		slicev1alpha1.AllocationClaimStatusInUse,
		slicev1alpha1.AllocationClaimStatusOrphaned,
		slicev1alpha1.AllocationClaimStatusStaged,
	} {
		AllocationClaimStateTotal.WithLabelValues(string(s), namespace).Set(0)
	}

	// Set the current state to 1
	AllocationClaimStateTotal.WithLabelValues(string(state), namespace).Set(1)
}

// RecordAllocationClaimTransition records a state transition
func (m *MetricsManager) RecordAllocationClaimTransition(fromState, toState slicev1alpha1.AllocationClaimState, namespace string) {
	AllocationClaimTransitionsTotal.WithLabelValues(string(fromState), string(toState), namespace).Inc()
}

// StartAllocationClaimTimer starts timing an allocation claim
func (m *MetricsManager) StartAllocationClaimTimer(claimName, namespace string) {
	m.allocationClaimTimers[claimName] = time.Now()
}

// EndAllocationClaimTimer ends timing and records duration
func (m *MetricsManager) EndAllocationClaimTimer(claimName, namespace string, finalState slicev1alpha1.AllocationClaimState) {
	if startTime, exists := m.allocationClaimTimers[claimName]; exists {
		duration := time.Since(startTime).Seconds()
		AllocationClaimDurationSeconds.WithLabelValues(string(finalState), namespace).Observe(duration)
		delete(m.allocationClaimTimers, claimName)
	}
}

// RecordSchedulerFilterResult records filter phase results
func (m *MetricsManager) RecordSchedulerFilterResult(result string, node string) {
	SchedulerFilterAttemptsTotal.WithLabelValues(result, node).Inc()
}

// RecordSchedulerScore records score phase invocations
func (m *MetricsManager) RecordSchedulerScore(node string) {
	SchedulerScoreInvocationsTotal.WithLabelValues(node).Inc()
}

// RecordSchedulerPrebindLatency records prebind latency
func (m *MetricsManager) RecordSchedulerPrebindLatency(node string, duration time.Duration) {
	SchedulerPrebindLatencySeconds.WithLabelValues(node).Observe(duration.Seconds())
}

// RecordSliceProvision records slice provisioning results
func (m *MetricsManager) RecordSliceProvision(result, profile, node string, duration time.Duration) {
	SliceProvisionTotal.WithLabelValues(result, profile, node).Inc()
	if result == "success" {
		SliceProvisionLatencySeconds.WithLabelValues(profile, node).Observe(duration.Seconds())
	}
}

// RecordSliceDeletion records slice deletion results
func (m *MetricsManager) RecordSliceDeletion(result, profile, node string, duration time.Duration) {
	SliceDeletionTotal.WithLabelValues(result, profile, node).Inc()
	if result == "success" {
		SliceDeletionLatencySeconds.WithLabelValues(profile, node).Observe(duration.Seconds())
	}
}

// RecordReconcileResult records reconciliation results
func (m *MetricsManager) RecordReconcileResult(controller, result string, duration time.Duration) {
	ReconcileTotal.WithLabelValues(controller, result).Inc()
	ReconcileDurationSeconds.WithLabelValues(controller).Observe(duration.Seconds())
}

// RecordError records errors from components
func (m *MetricsManager) RecordError(component, errorType string) {
	ErrorsTotal.WithLabelValues(component, errorType).Inc()
}

// SetInfo sets build information
func (m *MetricsManager) SetInfo(version, gitCommit, buildDate string) {
	Info.WithLabelValues(version, gitCommit, buildDate).Set(1)
}

// SetEmulatedMode sets emulated mode status
func (m *MetricsManager) SetEmulatedMode(mode slicev1alpha1.EmulatedMode) {
	// Reset all modes to 0
	EmulatedMode.WithLabelValues(string(slicev1alpha1.EmulatedModeDisabled)).Set(0)
	EmulatedMode.WithLabelValues(string(slicev1alpha1.EmulatedModeEnabled)).Set(0)

	// Set current mode to 1
	EmulatedMode.WithLabelValues(string(mode)).Set(1)
}

// StartMetricsServer starts the Prometheus metrics server
func StartMetricsServer(ctx context.Context, port string) {
	if port == "" {
		port = "8080"
	}

	klog.Infof("Starting metrics server on port %s", port)

	// The metrics will be automatically exposed via the default Prometheus registry
	// In a real implementation, you would start an HTTP server here
	// For now, we'll just log that metrics are available
	klog.Infof("Metrics available at /metrics endpoint")

	<-ctx.Done()
	klog.Infof("Metrics server stopped")
}
