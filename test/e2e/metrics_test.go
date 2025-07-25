package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	metricsTestNamespace = "das-e2e-metrics"
	metricsPort          = "8080"
)

var _ = Describe("Metrics Endpoint Tests", Ordered, func() {
	var (
		metricsClient *http.Client
	)

	BeforeAll(func() {
		if os.Getenv("KUBECONFIG") == "" {
			Skip("KUBECONFIG is not set; skipping metrics e2e test")
		}

		// Create HTTP client for metrics requests
		metricsClient = &http.Client{
			Timeout: 10 * time.Second,
		}

		// Create test namespace
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: metricsTestNamespace}}
		By("creating namespace " + metricsTestNamespace)
		_, err := kubeClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterAll(func() {
		By("deleting namespace " + metricsTestNamespace)
		err := kubeClient.CoreV1().Namespaces().Delete(context.Background(), metricsTestNamespace, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			_, err := kubeClient.CoreV1().Namespaces().Get(context.Background(), metricsTestNamespace, metav1.GetOptions{})
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, time.Second).Should(BeTrue())
	})

	It("should expose metrics endpoint on operator pods", func(ctx SpecContext) {
		By("finding operator pods")
		pods, err := kubeClient.CoreV1().Pods("das-operator").List(ctx, metav1.ListOptions{
			LabelSelector: "app=das-operator",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(pods.Items)).To(BeNumerically(">", 0))

		By("checking metrics endpoint on each operator pod")
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			metricsURL := fmt.Sprintf("http://localhost:8001/api/v1/namespaces/das-operator/pods/%s:%s/proxy/metrics", pod.Name, metricsPort)

			Eventually(func() error {
				resp, err := metricsClient.Get(metricsURL)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
				}

				// Read response body
				body := make([]byte, 1024*1024) // 1MB buffer
				n, err := resp.Body.Read(body)
				if err != nil && err.Error() != "EOF" {
					return err
				}
				bodyStr := string(body[:n])

				// Check for required metrics
				requiredMetrics := []string{
					"das_instaslice_info",
					"das_instaslice_emulated_mode",
					"das_instaslice_reconcile_total",
					"das_instaslice_errors_total",
				}

				for _, metric := range requiredMetrics {
					if !strings.Contains(bodyStr, metric) {
						return fmt.Errorf("metric %s not found in response", metric)
					}
				}

				return nil
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		}
	})

	It("should expose health endpoint on operator pods", func(ctx SpecContext) {
		By("finding operator pods")
		pods, err := kubeClient.CoreV1().Pods("das-operator").List(ctx, metav1.ListOptions{
			LabelSelector: "app=das-operator",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(pods.Items)).To(BeNumerically(">", 0))

		By("checking health endpoint on each operator pod")
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			healthURL := fmt.Sprintf("http://localhost:8001/api/v1/namespaces/das-operator/pods/%s:%s/proxy/health", pod.Name, metricsPort)

			Eventually(func() error {
				resp, err := metricsClient.Get(healthURL)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
				}

				// Read response body
				body := make([]byte, 1024)
				n, err := resp.Body.Read(body)
				if err != nil && err.Error() != "EOF" {
					return err
				}
				bodyStr := string(body[:n])

				if !strings.Contains(bodyStr, "OK") {
					return fmt.Errorf("expected 'OK' in response, got '%s'", bodyStr)
				}

				return nil
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		}
	})

	It("should expose metrics endpoint on scheduler pods", func(ctx SpecContext) {
		By("finding scheduler pods")
		pods, err := kubeClient.CoreV1().Pods("das-operator").List(ctx, metav1.ListOptions{
			LabelSelector: "app=das-scheduler",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(pods.Items)).To(BeNumerically(">", 0))

		By("checking metrics endpoint on each scheduler pod")
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			metricsURL := fmt.Sprintf("http://localhost:8001/api/v1/namespaces/das-operator/pods/%s:%s/proxy/metrics", pod.Name, metricsPort)

			Eventually(func() error {
				resp, err := metricsClient.Get(metricsURL)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
				}

				// Read response body
				body := make([]byte, 1024*1024) // 1MB buffer
				n, err := resp.Body.Read(body)
				if err != nil && err.Error() != "EOF" {
					return err
				}
				bodyStr := string(body[:n])

				// Check for scheduler-specific metrics
				schedulerMetrics := []string{
					"das_instaslice_scheduler_filter_attempts_total",
					"das_instaslice_scheduler_score_invocations_total",
					"das_instaslice_scheduler_prebind_latency_seconds",
				}

				for _, metric := range schedulerMetrics {
					if !strings.Contains(bodyStr, metric) {
						return fmt.Errorf("scheduler metric %s not found in response", metric)
					}
				}

				return nil
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		}
	})

	It("should expose metrics endpoint on daemonset pods", func(ctx SpecContext) {
		By("finding daemonset pods")
		pods, err := kubeClient.CoreV1().Pods("das-operator").List(ctx, metav1.ListOptions{
			LabelSelector: "app=das-daemonset",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(pods.Items)).To(BeNumerically(">", 0))

		By("checking metrics endpoint on each daemonset pod")
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			metricsURL := fmt.Sprintf("http://localhost:8001/api/v1/namespaces/das-operator/pods/%s:%s/proxy/metrics", pod.Name, metricsPort)

			Eventually(func() error {
				resp, err := metricsClient.Get(metricsURL)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
				}

				// Read response body
				body := make([]byte, 1024*1024) // 1MB buffer
				n, err := resp.Body.Read(body)
				if err != nil && err.Error() != "EOF" {
					return err
				}
				bodyStr := string(body[:n])

				// Check for device plugin specific metrics
				devicePluginMetrics := []string{
					"das_instaslice_slice_provision_total",
					"das_instaslice_slice_provision_latency_seconds",
					"das_instaslice_slice_deletion_total",
					"das_instaslice_slice_deletion_latency_seconds",
				}

				for _, metric := range devicePluginMetrics {
					if !strings.Contains(bodyStr, metric) {
						return fmt.Errorf("device plugin metric %s not found in response", metric)
					}
				}

				return nil
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		}
	})

	It("should record metrics during pod scheduling and allocation", func(ctx SpecContext) {
		By("creating a test pod to trigger metrics")
		podSpec := defaultGPUSlicePodSpec()
		testPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics-test-pod",
				Namespace: metricsTestNamespace,
			},
			Spec: podSpec,
		}

		_, err := kubeClient.CoreV1().Pods(metricsTestNamespace).Create(ctx, testPod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("waiting for pod to be scheduled and running")
		Eventually(func() (corev1.PodPhase, error) {
			pod, err := kubeClient.CoreV1().Pods(metricsTestNamespace).Get(ctx, "metrics-test-pod", metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			return pod.Status.Phase, nil
		}, 5*time.Minute, 10*time.Second).Should(Equal(corev1.PodRunning))

		By("checking that allocation claim was created")
		Eventually(func() (int, error) {
			allocs, err := dasClient.OpenShiftOperatorV1alpha1().AllocationClaims("das-operator").List(ctx, metav1.ListOptions{})
			if err != nil {
				return 0, err
			}
			return len(allocs.Items), nil
		}, 2*time.Minute, 5*time.Second).Should(BeNumerically(">", 0))

		By("verifying metrics were recorded")
		// Get operator pod to check metrics
		pods, err := kubeClient.CoreV1().Pods("das-operator").List(ctx, metav1.ListOptions{
			LabelSelector: "app=das-operator",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(pods.Items)).To(BeNumerically(">", 0))

		operatorPod := pods.Items[0]
		metricsURL := fmt.Sprintf("http://localhost:8001/api/v1/namespaces/das-operator/pods/%s:%s/proxy/metrics", operatorPod.Name, metricsPort)

		Eventually(func() error {
			resp, err := metricsClient.Get(metricsURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
			}

			// Read response body
			body := make([]byte, 1024*1024) // 1MB buffer
			n, err := resp.Body.Read(body)
			if err != nil && err.Error() != "EOF" {
				return err
			}
			bodyStr := string(body[:n])

			// Check for metrics that should be incremented during pod scheduling
			expectedMetrics := []string{
				"das_instaslice_allocationclaim_state_total",
				"das_instaslice_allocationclaim_transitions_total",
				"das_instaslice_reconcile_total",
			}

			for _, metric := range expectedMetrics {
				if !strings.Contains(bodyStr, metric) {
					return fmt.Errorf("metric %s not found in response", metric)
				}
			}

			return nil
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		By("cleaning up test pod")
		err = kubeClient.CoreV1().Pods(metricsTestNamespace).Delete(ctx, "metrics-test-pod", metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should have proper metric labels and values", func(ctx SpecContext) {
		By("finding operator pods")
		pods, err := kubeClient.CoreV1().Pods("das-operator").List(ctx, metav1.ListOptions{
			LabelSelector: "app=das-operator",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(pods.Items)).To(BeNumerically(">", 0))

		operatorPod := pods.Items[0]
		metricsURL := fmt.Sprintf("http://localhost:8001/api/v1/namespaces/das-operator/pods/%s:%s/proxy/metrics", operatorPod.Name, metricsPort)

		By("checking metric format and labels")
		Eventually(func() error {
			resp, err := metricsClient.Get(metricsURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
			}

			// Read response body
			body := make([]byte, 1024*1024) // 1MB buffer
			n, err := resp.Body.Read(body)
			if err != nil && err.Error() != "EOF" {
				return err
			}
			bodyStr := string(body[:n])

			// Check for proper metric format with labels
			expectedPatterns := []string{
				`das_instaslice_info{`,
				`das_instaslice_emulated_mode{`,
				`das_instaslice_reconcile_total{`,
				`das_instaslice_errors_total{`,
			}

			for _, pattern := range expectedPatterns {
				if !strings.Contains(bodyStr, pattern) {
					return fmt.Errorf("expected pattern %s not found in metrics", pattern)
				}
			}

			// Check that info metric has proper labels
			if !strings.Contains(bodyStr, `version=`) || !strings.Contains(bodyStr, `git_commit=`) {
				return fmt.Errorf("info metric missing expected labels")
			}

			// Check that emulated mode metric has proper labels
			if !strings.Contains(bodyStr, `mode=`) {
				return fmt.Errorf("emulated mode metric missing expected labels")
			}

			return nil
		}, 30*time.Second, 5*time.Second).Should(Succeed())
	})
})
