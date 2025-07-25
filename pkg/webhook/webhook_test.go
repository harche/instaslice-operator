package webhook

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	envNvidia = "NVIDIA_VISIBLE_DEVICES"
	envCUDA   = "CUDA_VISIBLE_DEVICES"
	testName  = "test"
)

func TestMutatePodNvidiaResource(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: testName},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  testName,
					Image: "ubuntu:20.04",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}

	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("failed to unmarshal mutated pod: %v", err)
	}

	if mutated.Spec.SchedulerName != secondaryScheduler {
		t.Fatalf("expected scheduler %s, got %s", secondaryScheduler, mutated.Spec.SchedulerName)
	}

	limits := mutated.Spec.Containers[0].Resources.Limits
	if _, ok := limits[corev1.ResourceName("nvidia.com/mig-1g.5gb")]; ok {
		t.Fatalf("nvidia resource still present")
	}
	q, ok := limits[corev1.ResourceName("mig.das.com/1g.5gb")]
	if !ok || q.Value() != 1 {
		t.Fatalf("expected instaslice resource quantity 1")
	}

	envs := mutated.Spec.Containers[0].Env
	var nvidiaEnv, cudaEnv bool
	for _, e := range envs {
		if e.Name == envNvidia {
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != testName || e.ValueFrom.ConfigMapKeyRef.Key != envNvidia {
				t.Fatalf("invalid NVIDIA_VISIBLE_DEVICES env")
			}
			nvidiaEnv = true
		}
		if e.Name == envCUDA {
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != testName || e.ValueFrom.ConfigMapKeyRef.Key != envCUDA {
				t.Fatalf("invalid CUDA_VISIBLE_DEVICES env")
			}
			cudaEnv = true
		}
	}
	if !nvidiaEnv || !cudaEnv {
		t.Fatalf("expected env vars not found")
	}
}

func TestMutatePodEphemeralNvidiaResource(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ephem"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main"}},
			EphemeralContainers: []corev1.EphemeralContainer{
				{
					EphemeralContainerCommon: corev1.EphemeralContainerCommon{
						Name:  "debug",
						Image: "busybox",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("1"),
							},
						},
					},
				},
			},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}

	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("unmarshal mutated pod: %v", err)
	}
	if mutated.Spec.SchedulerName != secondaryScheduler {
		t.Fatalf("expected scheduler set")
	}
	limits := mutated.Spec.EphemeralContainers[0].Resources.Limits
	if _, ok := limits[corev1.ResourceName("mig.das.com/1g.5gb")]; !ok {
		t.Fatalf("instaslice resource missing")
	}

	envs := mutated.Spec.EphemeralContainers[0].Env
	var nvidiaEnv, cudaEnv bool
	for _, e := range envs {
		if e.Name == envNvidia {
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != "ephem" || e.ValueFrom.ConfigMapKeyRef.Key != envNvidia {
				t.Fatalf("invalid NVIDIA_VISIBLE_DEVICES env")
			}
			nvidiaEnv = true
		}
		if e.Name == envCUDA {
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != "ephem" || e.ValueFrom.ConfigMapKeyRef.Key != envCUDA {
				t.Fatalf("invalid CUDA_VISIBLE_DEVICES env")
			}
			cudaEnv = true
		}
	}
	if !nvidiaEnv || !cudaEnv {
		t.Fatalf("expected env vars not found")
	}
}

func TestMutatePodOverrideValues(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "override"},
		Spec: corev1.PodSpec{
			SchedulerName: "foo",
			Containers: []corev1.Container{
				{
					Name:  "c",
					Image: "busybox",
					Env: []corev1.EnvVar{
						{Name: envNvidia, Value: "0"},
						{Name: envCUDA, Value: "0"},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}

	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("unmarshal mutated pod: %v", err)
	}

	if mutated.Spec.SchedulerName != secondaryScheduler {
		t.Fatalf("scheduler not overridden")
	}

	envs := mutated.Spec.Containers[0].Env
	if len(envs) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(envs))
	}
	for _, e := range envs {
		switch e.Name {
		case envNvidia:
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != "override" || e.ValueFrom.ConfigMapKeyRef.Key != envNvidia {
				t.Fatalf("NVIDIA env not correctly set")
			}
		case envCUDA:
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != "override" || e.ValueFrom.ConfigMapKeyRef.Key != envCUDA {
				t.Fatalf("CUDA env not correctly set")
			}
		default:
			t.Fatalf("unexpected env var %s", e.Name)
		}
	}
}

func TestMutatePodInstaResource(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: testName},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  testName,
					Image: "ubuntu:20.04",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName("mig.das.com/1g.5gb"): resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}
	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("unmarshal mutated pod: %v", err)
	}
	if mutated.Spec.SchedulerName != secondaryScheduler {
		t.Fatalf("expected scheduler set")
	}
	if _, ok := mutated.Spec.Containers[0].Resources.Limits[corev1.ResourceName("mig.das.com/1g.5gb")]; !ok {
		t.Fatalf("instaslice resource missing")
	}

	envs := mutated.Spec.Containers[0].Env
	var nvidiaEnv, cudaEnv bool
	for _, e := range envs {
		if e.Name == envNvidia {
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != testName || e.ValueFrom.ConfigMapKeyRef.Key != envNvidia {
				t.Fatalf("invalid NVIDIA_VISIBLE_DEVICES env")
			}
			nvidiaEnv = true
		}
		if e.Name == envCUDA {
			if e.ValueFrom == nil || e.ValueFrom.ConfigMapKeyRef == nil || e.ValueFrom.ConfigMapKeyRef.Name != testName || e.ValueFrom.ConfigMapKeyRef.Key != envCUDA {
				t.Fatalf("invalid CUDA_VISIBLE_DEVICES env")
			}
			cudaEnv = true
		}
	}
	if !nvidiaEnv || !cudaEnv {
		t.Fatalf("expected env vars not found")
	}
}

func TestMutatePodNoResource(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "none"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "t"}},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}
	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("unmarshal mutated pod: %v", err)
	}
	if mutated.Spec.SchedulerName != "" {
		t.Fatalf("expected scheduler not set")
	}
	if len(mutated.Spec.Containers[0].Env) != 0 {
		t.Fatalf("env vars should not be added")
	}
}

func TestMutatePodGPUMemoryInjection(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-mem-test"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "ubuntu:20.04",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("2"),
						},
					},
				},
			},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}

	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("failed to unmarshal mutated pod: %v", err)
	}

	if mutated.Spec.SchedulerName != secondaryScheduler {
		t.Fatalf("expected scheduler %s, got %s", secondaryScheduler, mutated.Spec.SchedulerName)
	}

	limits := mutated.Spec.Containers[0].Resources.Limits

	// Check that MIG resource was renamed
	if _, ok := limits[corev1.ResourceName("nvidia.com/mig-1g.5gb")]; ok {
		t.Fatalf("nvidia resource still present")
	}

	// Check that mig.das.com resource was added
	migResource, ok := limits[corev1.ResourceName("mig.das.com/1g.5gb")]
	if !ok || migResource.Value() != 2 {
		t.Fatalf("expected mig.das.com resource with quantity 2, got %v", migResource)
	}

	// Check that GPU memory resource was injected (5GB * 2 = 10GB)
	gpuMemResource, ok := limits[corev1.ResourceName("gpu.das.com/mem")]
	if !ok || gpuMemResource.Value() != 10 {
		t.Fatalf("expected gpu.das.com/mem resource with quantity 10, got %v", gpuMemResource)
	}
}

func TestMutatePodDirectGPUMemoryRequest(t *testing.T) {
	hook := &InstasliceWebhook{}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "direct-gpu-mem-test"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName("gpu.das.com/mem"): resource.MustParse("10"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceName("gpu.das.com/mem"): resource.MustParse("10"),
						},
					},
				},
			},
		},
	}

	data, err := hook.mutatePod(pod)
	if err != nil {
		t.Fatalf("mutatePod returned error: %v", err)
	}

	mutated := &corev1.Pod{}
	if err := json.Unmarshal(data, mutated); err != nil {
		t.Fatalf("failed to unmarshal mutated pod: %v", err)
	}

	limits := mutated.Spec.Containers[0].Resources.Limits

	// Check that the MIG resource was added and mapped correctly
	migResource, ok := limits[corev1.ResourceName("mig.das.com/2g.10gb")]
	if !ok || migResource.Value() != 10 {
		t.Fatalf("expected mig.das.com/2g.10gb resource with quantity 10, got %v", migResource)
	}

	// Check that the original gpu.das.com/mem resource is not present
	if _, ok := limits[corev1.ResourceName("gpu.das.com/mem")]; ok {
		t.Fatalf("gpu.das.com/mem resource should have been mapped and removed")
	}
}

// mapGPUMemoryToMIGProfile maps a GPU memory request in GB to the most suitable MIG profile
// This function implements a simple mapping strategy - you can enhance it based on your needs
func mapGPUMemoryToMIGProfile(memGB int64) string {
	// Define available MIG profiles and their memory requirements
	// This mapping can be enhanced to be more sophisticated or configurable
	switch {
	case memGB <= 5:
		return "1g.5gb"
	case memGB <= 10:
		return "2g.10gb"
	case memGB <= 20:
		return "3g.20gb"
	case memGB <= 40:
		return "7g.40gb"
	case memGB <= 80:
		return "7g.80gb"
	default:
		// For very large memory requests, return empty string to indicate no suitable profile
		return ""
	}
}

// extractGPUMemoryFromProfile extracts the GPU memory requirement from a MIG profile string
func extractGPUMemoryFromProfile(profile string) int64 {
	// This function is a placeholder. In a real scenario, you would parse the profile string
	// to determine the memory requirement.
	// For example, "1g.5gb" -> 5, "2g.10gb" -> 10, etc.
	// For now, we'll return a default or 0 if not recognized.
	switch profile {
	case "1g.5gb":
		return 5
	case "2g.10gb":
		return 10
	case "3g.20gb":
		return 20
	case "7g.40gb":
		return 40
	case "7g.80gb":
		return 80
	default:
		return 0
	}
}
