/*
Copyright 2025.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

type AllocationStatus string

const (
	AllocationStatusDeleted  AllocationStatus = "deleted"
	AllocationStatusDeleting AllocationStatus = "deleting"
	AllocationStatusUngated  AllocationStatus = "ungated"
	AllocationStatusCreating AllocationStatus = "creating"
	AllocationStatusCreated  AllocationStatus = "created"
)

type AllocationRequest struct {
	// profile specifies the MIG slice profile for allocation
	// +kubebuilder:validation:optional
	Profile string `json:"profile"`

	// resources specifies resource requirements for the allocation
	// +kubebuilder:validation:optional
	Resources corev1.ResourceRequirements `json:"resources"`

	// podRef is a reference to the gated Pod requesting the allocation
	// +kubebuilder:validation:optional
	PodRef corev1.ObjectReference `json:"podRef"`
}

type AllocationResult struct {
	// migPlacement specifies the MIG placement details
	// +kubebuilder:validation:Required
	MigPlacement Placement `json:"migPlacement"`

	// gpuUUID represents the UUID of the selected GPU
	// +kubebuilder:validation:Required
	GPUUUID string `json:"gpuUUID"`

	// nodename represents the name of the selected node
	// +kubebuilder:validation:Required
	Nodename types.NodeName `json:"nodename"`

	// allocationStatus represents the current status of the allocation
	// +kubebuilder:validation:Enum=deleted;deleting;ungated;creating;created
	// +kubebuilder:validation:Required
	AllocationStatus AllocationStatus `json:"allocationStatus"`

	// configMapResourceIdentifier represents the UUID used for creating the ConfigMap resource
	// +kubebuilder:validation:Required
	ConfigMapResourceIdentifier types.UID `json:"configMapResourceIdentifier"`

	// conditions provide additional information about the allocation
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type DiscoveredGPU struct {
	// gpuUUID represents the UUID of the GPU
	// +kubebuilder:validation:Required
	GPUUUID string `json:"gpuUuid"`

	// gpuName represents the name of the GPU
	// +kubebuilder:validation:Required
	GPUName string `json:"gpuName"`

	// gpuMemory represents the memory capacity of the GPU
	// +kubebuilder:validation:Required
	GPUMemory resource.Quantity `json:"gpuMemory"`
}

type DiscoveredNodeResources struct {
	// nodeGPUs represents the discovered mig enabled GPUs on the node
	// +kubebuilder:validation:Required
	NodeGPUs []DiscoveredGPU `json:"nodeGpus"`

	// migPlacement represents GPU instance, compute instance with placement for a profile
	// +kubebuilder:validation:Required
	MigPlacement map[string]Mig `json:"migPlacement"`

	// nodeResources represents the resource list of the node at boot time
	// +kubebuilder:validation:Required
	NodeResources corev1.ResourceList `json:"nodeResources"`
}

type Mig struct {
	// placements specify vendor profile indexes and sizes
	// +kubebuilder:validation:Required
	Placements []Placement `json:"placements"`

	// giprofileID provides the GPU instance ID of a profile
	// +kubebuilder:validation:Required
	GIProfileID int32 `json:"giProfileId"`

	// ciProfileID provides the compute instance ID of a profile
	// +kubebuilder:validation:Required
	CIProfileID int32 `json:"ciProfileId"`

	// ciEngProfileID provides the compute instance engineering ID of a profile
	// +kubebuilder:validation:Optional
	CIEngProfileID int32 `json:"ciEngProfileId,omitempty"`
}

type Placement struct {
	// size represents slots consumed by a profile on GPU
	// +kubebuilder:validation:Required
	Size int32 `json:"size"`

	// start represents the starting index driven by size for a profile
	// +kubebuilder:validation:Required
	Start int32 `json:"start"`
}

type InstasliceSpec struct {
	// podAllocationRequests specifies the allocation requests per pod
	// +kubebuilder:validation:optional
	// +optional
	PodAllocationRequests map[types.UID]*AllocationRequest `json:"podAllocationRequests"`

	// emulatorMode specifies whether the Instaslice is running in emulator mode
	// +kubebuilder:validation:Optional
	EmulatorMode bool `json:"emulatorMode"`
}

type InstasliceStatus struct {
	// podAllocationResults specify the allocation results per pod
	// +kubebuilder:validation:optional
	// +optional
	PodAllocationResults map[types.UID]*AllocationResult `json:"podAllocationResults"`

	// nodeResources specifies the discovered resources of the node
	// +kubebuilder:validation:optional
	NodeResources DiscoveredNodeResources `json:"nodeResources"`

	// conditions represent the observed state of the Instaslice object
	// For example:
	//   conditions:
	//   - type: Ready
	//     status: "True"
	//     lastTransitionTime: "2025-01-22T12:34:56Z"
	//     reason: "GPUsAccessible"
	//     message: "All discovered GPUs are accessible and the driver is healthy."
	//
	// Or, in an error scenario (driver not responding):
	//   conditions:
	//   - type: Ready
	//     status: "False"
	//     lastTransitionTime: "2025-01-22T12:34:56Z"
	//     reason: "DriverError"
	//     message: "Could not communicate with the GPU driver on the node."
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Instaslice is the Schema for the instaslices API
// +kubebuilder:validation:Required
// +kubebuilder:subresource:status
type Instaslice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstasliceSpec   `json:"spec"`
	Status InstasliceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InstasliceList contains a list of Instaslice resources
// +kubebuilder:validation:Optional
type InstasliceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Instaslice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Instaslice{}, &InstasliceList{})
}
