package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openshift/instaslice-operator/pkg/metrics"
)

func TestMetricsServerIntegration(t *testing.T) {
	// Test metrics server creation and basic functionality
	t.Run("MetricsServerBasic", func(t *testing.T) {
		server := metrics.NewServer("8081")
		if server == nil {
			t.Fatal("Expected server to be created")
		}
		if server.GetPort() != "8081" {
			t.Errorf("Expected port 8081, got %s", server.GetPort())
		}

		// Test default port
		server = metrics.NewServer("")
		if server == nil {
			t.Fatal("Expected server to be created")
		}
		if server.GetPort() != "8080" {
			t.Errorf("Expected default port 8080, got %s", server.GetPort())
		}
	})

	// Test metrics server endpoints
	t.Run("MetricsServerEndpoints", func(t *testing.T) {
		server := metrics.NewServer("8082")
		if server == nil {
			t.Fatal("Expected server to be created")
		}

		// Start server in background
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			err := server.Start(ctx)
			if err != nil && err != context.Canceled {
				t.Errorf("Server error: %v", err)
			}
		}()

		// Wait for server to start
		time.Sleep(100 * time.Millisecond)

		// Test metrics endpoint
		t.Run("MetricsEndpoint", func(t *testing.T) {
			// Set some metrics before checking
			manager := metrics.NewMetricsManager()
			manager.SetInfo("v1.0.0", "test-commit", "2024-01-01")
			manager.SetEmulatedMode("disabled")
			manager.RecordError("test-component", "test-error")

			resp, err := http.Get("http://localhost:8082/metrics")
			if err != nil {
				t.Fatalf("Failed to get metrics: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			// Read response body
			body := make([]byte, 1024*1024) // 1MB buffer
			n, err := resp.Body.Read(body)
			if err != nil && err.Error() != "EOF" {
				t.Fatalf("Failed to read response body: %v", err)
			}
			bodyStr := string(body[:n])

			// Check for required metrics
			requiredMetrics := []string{
				"das_instaslice_info",
				"das_instaslice_emulated_mode",
				"das_instaslice_errors_total",
			}

			for _, metric := range requiredMetrics {
				if !strings.Contains(bodyStr, metric) {
					t.Errorf("Metric %s not found in response", metric)
				}
			}
		})

		// Test health endpoint
		t.Run("HealthEndpoint", func(t *testing.T) {
			resp, err := http.Get("http://localhost:8082/health")
			if err != nil {
				t.Fatalf("Failed to get health: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			// Read response body
			body := make([]byte, 1024)
			n, err := resp.Body.Read(body)
			if err != nil && err.Error() != "EOF" {
				t.Fatalf("Failed to read response body: %v", err)
			}
			bodyStr := string(body[:n])

			if !strings.Contains(bodyStr, "OK") {
				t.Errorf("Expected 'OK' in response, got '%s'", bodyStr)
			}
		})

		// Stop server
		cancel()
		time.Sleep(100 * time.Millisecond)
	})
}

func TestMetricsManagerIntegration(t *testing.T) {
	// Test metrics manager functionality
	t.Run("MetricsManagerBasic", func(t *testing.T) {
		manager := metrics.NewMetricsManager()
		if manager == nil {
			t.Fatal("Expected manager to be created")
		}

		// Test GPU inventory recording
		t.Run("GPUInventory", func(t *testing.T) {
			node := "test-node"
			gpuUUID := "test-gpu-uuid"
			profile := "1g.5gb"
			capacity := int32(7)
			allocated := int32(3)

			// This should not panic
			manager.RecordGPUInventory(node, gpuUUID, profile, capacity, allocated)
		})

		// Test AllocationClaim tracking
		t.Run("AllocationClaimTracking", func(t *testing.T) {
			claimName := "test-claim"
			namespace := "test-namespace"

			// Start timer
			manager.StartAllocationClaimTimer(claimName, namespace)

			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)

			// End timer
			manager.EndAllocationClaimTimer(claimName, namespace, "inUse")

			// Note: We can't easily test timer removal without exposing internal state
			// This test verifies the methods don't panic
		})

		// Test error recording
		t.Run("ErrorRecording", func(t *testing.T) {
			component := "test-component"
			errorType := "test-error"

			// This should not panic
			manager.RecordError(component, errorType)
		})
	})
}

func TestGlobalTrackersIntegration(t *testing.T) {
	// Test global tracker instances
	t.Run("GlobalTrackers", func(t *testing.T) {
		// Test AllocationClaim tracker
		t.Run("AllocationClaimTracker", func(t *testing.T) {
			if metrics.GlobalAllocationClaimTracker == nil {
				t.Fatal("Expected GlobalAllocationClaimTracker to be initialized")
			}

			claimName := "test-claim"
			namespace := "test-namespace"

			// These should not panic
			metrics.GlobalAllocationClaimTracker.TrackAllocationClaimCreation(claimName, namespace)
			metrics.GlobalAllocationClaimTracker.TrackAllocationClaimStateTransition("staged", "created", namespace)
			metrics.GlobalAllocationClaimTracker.TrackAllocationClaimCompletion(claimName, namespace, "inUse")
		})

		// Test GPU Inventory tracker
		t.Run("GPUInventoryTracker", func(t *testing.T) {
			if metrics.GlobalGPUInventoryTracker == nil {
				t.Fatal("Expected GlobalGPUInventoryTracker to be initialized")
			}

			node := "test-node"
			gpuUUID := "test-gpu-uuid"
			profile := "1g.5gb"
			capacity := int32(7)
			allocated := int32(3)

			// These should not panic
			metrics.GlobalGPUInventoryTracker.UpdateGPUInventory(node, gpuUUID, profile, capacity, allocated)
			metrics.GlobalGPUInventoryTracker.UpdateAllocationClaimState("inUse", "test-namespace")
		})

		// Test Scheduler tracker
		t.Run("SchedulerTracker", func(t *testing.T) {
			if metrics.GlobalSchedulerTracker == nil {
				t.Fatal("Expected GlobalSchedulerTracker to be initialized")
			}

			node := "test-node"
			result := "success"
			duration := 100 * time.Millisecond

			// These should not panic
			metrics.GlobalSchedulerTracker.TrackFilterResult(result, node)
			metrics.GlobalSchedulerTracker.TrackScoreInvocation(node)
			metrics.GlobalSchedulerTracker.TrackPrebindLatency(node, duration)
		})

		// Test Device Plugin tracker
		t.Run("DevicePluginTracker", func(t *testing.T) {
			if metrics.GlobalDevicePluginTracker == nil {
				t.Fatal("Expected GlobalDevicePluginTracker to be initialized")
			}

			result := "success"
			profile := "1g.5gb"
			node := "test-node"
			duration := 500 * time.Millisecond

			// These should not panic
			metrics.GlobalDevicePluginTracker.TrackSliceProvision(result, profile, node, duration)
			metrics.GlobalDevicePluginTracker.TrackSliceDeletion(result, profile, node, duration)
		})

		// Test Controller tracker
		t.Run("ControllerTracker", func(t *testing.T) {
			if metrics.GlobalControllerTracker == nil {
				t.Fatal("Expected GlobalControllerTracker to be initialized")
			}

			controller := "TestController"
			result := "success"
			duration := 200 * time.Millisecond
			component := "test-component"
			errorType := "test-error"

			// These should not panic
			metrics.GlobalControllerTracker.TrackReconcileResult(controller, result, duration)
			metrics.GlobalControllerTracker.TrackError(component, errorType)
		})
	})
}

func TestMetricsFormatIntegration(t *testing.T) {
	// Test metrics format and structure
	t.Run("MetricsFormat", func(t *testing.T) {
		server := metrics.NewServer("8083")
		if server == nil {
			t.Fatal("Expected server to be created")
		}

		// Start server in background
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			err := server.Start(ctx)
			if err != nil && err != context.Canceled {
				t.Errorf("Server error: %v", err)
			}
		}()

		// Wait for server to start
		time.Sleep(100 * time.Millisecond)

		// Set some metrics before checking
		manager := metrics.NewMetricsManager()
		manager.SetInfo("v1.0.0", "test-commit", "2024-01-01")
		manager.SetEmulatedMode("disabled")
		manager.RecordError("test-component", "test-error")

		// Test metrics format
		resp, err := http.Get("http://localhost:8083/metrics")
		if err != nil {
			t.Fatalf("Failed to get metrics: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		// Read response body
		body := make([]byte, 1024*1024) // 1MB buffer
		n, err := resp.Body.Read(body)
		if err != nil && err.Error() != "EOF" {
			t.Fatalf("Failed to read response body: %v", err)
		}
		bodyStr := string(body[:n])

		// Check for proper Prometheus format
		if !strings.Contains(bodyStr, "# HELP") {
			t.Error("Expected '# HELP' in metrics response")
		}
		if !strings.Contains(bodyStr, "# TYPE") {
			t.Error("Expected '# TYPE' in metrics response")
		}

		// Check for proper metric namespacing
		if !strings.Contains(bodyStr, "das_instaslice_") {
			t.Error("Expected 'das_instaslice_' namespace in metrics")
		}

		// Check for proper metric types
		if !strings.Contains(bodyStr, "# TYPE das_instaslice_info gauge") {
			t.Error("Expected gauge type for info metric")
		}

		// Check for proper labels
		if !strings.Contains(bodyStr, `{version=`) {
			t.Error("Expected version label in metrics")
		}
		if !strings.Contains(bodyStr, `{mode=`) {
			t.Error("Expected mode label in metrics")
		}

		// Stop server
		cancel()
		time.Sleep(100 * time.Millisecond)
	})
}
