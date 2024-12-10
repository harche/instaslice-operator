/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"github.com/openshift/instaslice-operator/api/v1alpha1"
	inferencev1alpha1 "github.com/openshift/instaslice-operator/api/v1alpha1"
	"github.com/openshift/instaslice-operator/internal/controller"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateFakeCapacity(nodeName string) *inferencev1alpha1.Instaslice {
	return &v1alpha1.Instaslice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: controller.InstaSliceOperatorNamespace,
		},
		Spec: v1alpha1.InstasliceSpec{
			CpuOnNodeAtBoot:    72,
			MemoryOnNodeAtBoot: 1000000000,
			MigGPUUUID: map[string]string{
				"GPU-8d042338-e67f-9c48-92b4-5b55c7e5133c": "NVIDIA A100-PCIE-40GB",
				"GPU-31cfe05c-ed13-cd17-d7aa-c63db5108c24": "NVIDIA A100-PCIE-40GB",
			},
			Migplacement: []v1alpha1.Mig{
				{
					CIProfileID:    0,
					CIEngProfileID: 0,
					Giprofileid:    0,
					Placements: []v1alpha1.Placement{
						{Size: 1, Start: 0},
						{Size: 1, Start: 1},
						{Size: 1, Start: 2},
						{Size: 1, Start: 3},
						{Size: 1, Start: 4},
						{Size: 1, Start: 5},
						{Size: 1, Start: 6},
					},
					Profile: "1g.5gb",
				},
				{
					CIProfileID:    1,
					CIEngProfileID: 0,
					Giprofileid:    1,
					Placements: []v1alpha1.Placement{
						{Size: 2, Start: 0},
						{Size: 2, Start: 2},
						{Size: 2, Start: 4},
					},
					Profile: "2g.10gb",
				},
				{
					CIProfileID:    2,
					CIEngProfileID: 0,
					Giprofileid:    2,
					Placements: []v1alpha1.Placement{
						{Size: 4, Start: 0},
						{Size: 4, Start: 4},
					},
					Profile: "3g.20gb",
				},
				{
					CIProfileID:    3,
					CIEngProfileID: 0,
					Giprofileid:    3,
					Placements: []v1alpha1.Placement{
						{Size: 4, Start: 0},
					},
					Profile: "4g.20gb",
				},
				{
					CIProfileID:    4,
					CIEngProfileID: 0,
					Giprofileid:    4,
					Placements: []v1alpha1.Placement{
						{Size: 8, Start: 0},
					},
					Profile: "7g.40gb",
				},
				{
					CIProfileID:    7,
					CIEngProfileID: 0,
					Giprofileid:    7,
					Placements: []v1alpha1.Placement{
						{Size: 1, Start: 0},
						{Size: 1, Start: 1},
						{Size: 1, Start: 2},
						{Size: 1, Start: 3},
						{Size: 1, Start: 4},
						{Size: 1, Start: 5},
						{Size: 1, Start: 6},
					},
					Profile: "1g.5gb+me",
				},
				{
					CIProfileID:    9,
					CIEngProfileID: 0,
					Giprofileid:    9,
					Placements: []v1alpha1.Placement{
						{Size: 2, Start: 0},
						{Size: 2, Start: 2},
						{Size: 2, Start: 4},
						{Size: 2, Start: 6},
					},
					Profile: "1g.10gb",
				},
			},
		},
		Status: v1alpha1.InstasliceStatus{
			Processed: true,
		},
	}
}

func GetVectorAddFinalizerPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vectoradd-finalizer",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:  "vectoradd-finalizer",
					Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
						},
					},
					Command: []string{"sh", "-c", "sleep 20"},
				},
			},
		},
	}
}

func GetVectorAddNoReqPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vectoradd-no-req",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:  "vectoradd-no-req",
					Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
						},
					},
					Command: []string{"sh", "-c", "sleep 20"},
				},
			},
		},
	}
}

func GetVectorAddSmallReqPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vectoradd-small-req",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:  "vectoradd-small-req",
					Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
						},
					},
					Command: []string{"sh", "-c", "sleep 20"},
				},
			},
		},
	}
}

func GetVectorAddLargeMemPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vectoradd-large-mem",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:  "vectoradd-large-mem",
					Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1000000000000000Mi"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
						},
					},
					Command: []string{"sh", "-c", "sleep 20"},
				},
			},
		},
	}
}

func GetVectorAddLargeCPUPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vectoradd-large-cpu",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			Containers: []corev1.Container{
				{
					Name:  "vectoradd-large-cpu",
					Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("5000000000000m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Limits: corev1.ResourceList{
							"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
						},
					},
					Command: []string{"sh", "-c", "sleep 20"},
				},
			},
		},
	}
}

func GetSleepDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sleep-deployment",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sleep-app",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func(i int32) *int32 { return &i }(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "sleep-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "sleep-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "sleep-container",
							Image: "busybox",
							Command: []string{
								"/bin/sh", "-c",
							},
							Args: []string{
								"sleep 3600",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:      resource.MustParse("100m"),
									corev1.ResourceMemory:   resource.MustParse("64Mi"),
									"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"cat", "/tmp/healthy"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}
}

func GetSleepStatefulSet() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sleep-statefulset",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sleep-app",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "sleep-service",
			Replicas:    func(i int32) *int32 { return &i }(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "sleep-stateful",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "sleep-stateful",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "sleep-container",
							Image: "busybox",
							Command: []string{
								"/bin/sh",
								"-c",
							},
							Args: []string{
								"sleep 3600",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:      resource.MustParse("100m"),
									corev1.ResourceMemory:   resource.MustParse("64Mi"),
									"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func GetSleepJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sleep-job",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sleep-job",
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "sleep-job",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "sleep-container",
							Image: "busybox",
							Command: []string{
								"/bin/sh",
								"-c",
							},
							Args: []string{
								"sleep 3600",
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:      resource.MustParse("100m"),
									corev1.ResourceMemory:   resource.MustParse("64Mi"),
									"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

func GetMultiPods() []*corev1.Pod {
	podNames := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7"}
	labels := map[string]string{"kueue.x-k8s.io/queue-name": "mig-queue"}
	pods := make([]*corev1.Pod, 0, len(podNames))

	for _, name := range podNames {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
				Labels:    labels,
			},
			Spec: corev1.PodSpec{
				RestartPolicy:                 corev1.RestartPolicyNever,
				TerminationGracePeriodSeconds: func(i int64) *int64 { return &i }(0),
				Containers: []corev1.Container{
					{
						Name:  "vectoradd",
						Image: "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0-ubi8",
						Command: []string{
							"sh", "-c", "sleep 20",
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"nvidia.com/mig-1g.5gb": resource.MustParse("1"),
							},
						},
					},
				},
			},
		}

		pods = append(pods, pod)
	}
	return pods
}