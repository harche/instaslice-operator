package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	slicev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/dasoperator/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistration(t *testing.T) {
	// Test that all metrics are properly defined
	expectedMetrics := []string{
		"das_instaslice_gpu_total",
		"das_instaslice_gpu_slice_capacity",
		"das_instaslice_gpu_slice_allocated",
		"das_instaslice_gpu_slice_free",
		"das_instaslice_allocationclaim_state_total",
		"das_instaslice_allocationclaim_transitions_total",
		"das_instaslice_allocationclaim_duration_seconds",
		"das_instaslice_scheduler_filter_attempts_total",
		"das_instaslice_scheduler_score_invocations_total",
		"das_instaslice_scheduler_prebind_latency_seconds",
		"das_instaslice_slice_provision_total",
		"das_instaslice_slice_provision_latency_seconds",
		"das_instaslice_slice_deletion_total",
		"das_instaslice_slice_deletion_latency_seconds",
		"das_instaslice_reconcile_total",
		"das_instaslice_reconcile_duration_seconds",
		"das_instaslice_errors_total",
		"das_instaslice_info",
		"das_instaslice_emulated_mode",
	}

	// Verify metrics are defined (not nil)
	metrics := []prometheus.Collector{
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
	}

	for i, metric := range metrics {
		if metric == nil {
			t.Errorf("Expected metric %s to be defined", expectedMetrics[i])
		}
	}
}

func TestMetricsManager(t *testing.T) {
	manager := NewMetricsManager()

	// Test GPU inventory recording
	t.Run("RecordGPUInventory", func(t *testing.T) {
		node := "test-node"
		gpuUUID := "test-gpu-uuid"
		profile := "1g.5gb"
		capacity := int32(7)
		allocated := int32(3)

		// This should not panic
		manager.RecordGPUInventory(node, gpuUUID, profile, capacity, allocated)
	})

	// Test AllocationClaim state recording
	t.Run("RecordAllocationClaimState", func(t *testing.T) {
		namespace := "test-namespace"
		state := slicev1alpha1.AllocationClaimStatusInUse

		// This should not panic
		manager.RecordAllocationClaimState(state, namespace)
	})

	// Test AllocationClaim transition recording
	t.Run("RecordAllocationClaimTransition", func(t *testing.T) {
		fromState := slicev1alpha1.AllocationClaimStatusStaged
		toState := slicev1alpha1.AllocationClaimStatusCreated
		namespace := "test-namespace"

		// This should not panic
		manager.RecordAllocationClaimTransition(fromState, toState, namespace)
	})

	// Test AllocationClaim timer functionality
	t.Run("AllocationClaimTimer", func(t *testing.T) {
		claimName := "test-claim"
		namespace := "test-namespace"
		finalState := slicev1alpha1.AllocationClaimStatusInUse

		manager.StartAllocationClaimTimer(claimName, namespace)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure measurable duration
		manager.EndAllocationClaimTimer(claimName, namespace, finalState)

		// Verify timer was removed
		if _, exists := manager.allocationClaimTimers[claimName]; exists {
			t.Error("Timer should have been removed after completion")
		}
	})

	// Test scheduler metrics
	t.Run("SchedulerMetrics", func(t *testing.T) {
		node := "test-node"
		result := "success"

		// These should not panic
		manager.RecordSchedulerFilterResult(result, node)
		manager.RecordSchedulerScore(node)
		manager.RecordSchedulerPrebindLatency(node, 100*time.Millisecond)
	})

	// Test slice provisioning metrics
	t.Run("SliceProvisionMetrics", func(t *testing.T) {
		result := "success"
		profile := "1g.5gb"
		node := "test-node"
		duration := 500 * time.Millisecond

		// These should not panic
		manager.RecordSliceProvision(result, profile, node, duration)
		manager.RecordSliceDeletion(result, profile, node, duration)
	})

	// Test reconcile metrics
	t.Run("ReconcileMetrics", func(t *testing.T) {
		controller := "TestController"
		result := "success"
		duration := 200 * time.Millisecond

		// This should not panic
		manager.RecordReconcileResult(controller, result, duration)
	})

	// Test error metrics
	t.Run("ErrorMetrics", func(t *testing.T) {
		component := "test-component"
		errorType := "test-error"

		// This should not panic
		manager.RecordError(component, errorType)
	})

	// Test info metrics
	t.Run("InfoMetrics", func(t *testing.T) {
		version := "v1.0.0"
		gitCommit := "abc123"
		buildDate := "2024-01-15T10:30:00Z"

		// This should not panic
		manager.SetInfo(version, gitCommit, buildDate)
	})

	// Test emulated mode metrics
	t.Run("EmulatedModeMetrics", func(t *testing.T) {
		mode := slicev1alpha1.EmulatedMode(slicev1alpha1.EmulatedModeEnabled)

		// This should not panic
		manager.SetEmulatedMode(mode)
	})
}

func TestMetricsServer(t *testing.T) {
	// Test server creation
	t.Run("NewServer", func(t *testing.T) {
		server := NewServer("8080")
		if server == nil {
			t.Fatal("Expected server to be created")
		}
		if server.GetPort() != "8080" {
			t.Errorf("Expected port 8080, got %s", server.GetPort())
		}

		// Test default port
		server = NewServer("")
		if server.GetPort() != "8080" {
			t.Errorf("Expected default port 8080, got %s", server.GetPort())
		}
	})

	// Test metrics endpoint
	t.Run("MetricsEndpoint", func(t *testing.T) {
		server := NewServer("8080")

		// Create a test request
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		// Get the handler from the server
		handler := server.server.Handler
		handler.ServeHTTP(w, req)

		// Check response
		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		// Check that metrics are present
		body := w.Body.String()
		if !strings.Contains(body, "das_instaslice_info") {
			t.Error("Expected metrics to contain das_instaslice_info")
		}
	})

	// Test health endpoint
	t.Run("HealthEndpoint", func(t *testing.T) {
		server := NewServer("8080")

		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		handler := server.server.Handler
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if w.Body.String() != "OK" {
			t.Errorf("Expected body 'OK', got '%s'", w.Body.String())
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	// Test AllocationClaimTracker
	t.Run("AllocationClaimTracker", func(t *testing.T) {
		tracker := NewAllocationClaimTracker()
		if tracker == nil {
			t.Fatal("Expected tracker to be created")
		}

		claimName := "test-claim"
		namespace := "test-namespace"
		fromState := slicev1alpha1.AllocationClaimStatusStaged
		toState := slicev1alpha1.AllocationClaimStatusCreated
		finalState := slicev1alpha1.AllocationClaimStatusInUse

		// These should not panic
		tracker.TrackAllocationClaimCreation(claimName, namespace)
		tracker.TrackAllocationClaimStateTransition(fromState, toState, namespace)
		tracker.TrackAllocationClaimCompletion(claimName, namespace, finalState)
	})

	// Test GPUInventoryTracker
	t.Run("GPUInventoryTracker", func(t *testing.T) {
		tracker := NewGPUInventoryTracker()
		if tracker == nil {
			t.Fatal("Expected tracker to be created")
		}

		node := "test-node"
		gpuUUID := "test-gpu-uuid"
		profile := "1g.5gb"
		capacity := int32(7)
		allocated := int32(3)
		state := slicev1alpha1.AllocationClaimStatusInUse
		namespace := "test-namespace"

		// These should not panic
		tracker.UpdateGPUInventory(node, gpuUUID, profile, capacity, allocated)
		tracker.UpdateAllocationClaimState(state, namespace)
	})

	// Test SchedulerMetricsTracker
	t.Run("SchedulerMetricsTracker", func(t *testing.T) {
		tracker := NewSchedulerMetricsTracker()
		if tracker == nil {
			t.Fatal("Expected tracker to be created")
		}

		node := "test-node"
		result := "success"
		duration := 100 * time.Millisecond

		// These should not panic
		tracker.TrackFilterResult(result, node)
		tracker.TrackScoreInvocation(node)
		tracker.TrackPrebindLatency(node, duration)
	})

	// Test DevicePluginMetricsTracker
	t.Run("DevicePluginMetricsTracker", func(t *testing.T) {
		tracker := NewDevicePluginMetricsTracker()
		if tracker == nil {
			t.Fatal("Expected tracker to be created")
		}

		result := "success"
		profile := "1g.5gb"
		node := "test-node"
		duration := 500 * time.Millisecond

		// These should not panic
		tracker.TrackSliceProvision(result, profile, node, duration)
		tracker.TrackSliceDeletion(result, profile, node, duration)
	})

	// Test ControllerMetricsTracker
	t.Run("ControllerMetricsTracker", func(t *testing.T) {
		tracker := NewControllerMetricsTracker()
		if tracker == nil {
			t.Fatal("Expected tracker to be created")
		}

		controller := "TestController"
		result := "success"
		duration := 200 * time.Millisecond
		component := "test-component"
		errorType := "test-error"

		// These should not panic
		tracker.TrackReconcileResult(controller, result, duration)
		tracker.TrackError(component, errorType)
	})

	// Test global tracker instances
	t.Run("GlobalTrackers", func(t *testing.T) {
		if GlobalAllocationClaimTracker == nil {
			t.Error("Expected GlobalAllocationClaimTracker to be initialized")
		}
		if GlobalGPUInventoryTracker == nil {
			t.Error("Expected GlobalGPUInventoryTracker to be initialized")
		}
		if GlobalSchedulerTracker == nil {
			t.Error("Expected GlobalSchedulerTracker to be initialized")
		}
		if GlobalDevicePluginTracker == nil {
			t.Error("Expected GlobalDevicePluginTracker to be initialized")
		}
		if GlobalControllerTracker == nil {
			t.Error("Expected GlobalControllerTracker to be initialized")
		}
	})
}
