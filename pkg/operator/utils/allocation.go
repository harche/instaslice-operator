package utils

import (
	"context"
	"fmt"

	inferencev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
	clientset "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// AllocationPolicy interface for different allocation strategies
type AllocationPolicy interface {
	SetAllocationDetails(profileName string, newStart, size int32, podUUID types.UID, nodename types.NodeName, 
		allocationStatus inferencev1alpha1.AllocationStatus, discoveredGiprofile int32, Ciprofileid int32, 
		Ciengprofileid int32, namespace string, podName string, gpuUuid string, resourceIdentifier types.UID) (*inferencev1alpha1.AllocationRequest, *inferencev1alpha1.AllocationResult)
}

// FirstFitPolicy implements first-fit allocation strategy
type FirstFitPolicy struct{}

// SetAllocationDetails implements AllocationPolicy for FirstFitPolicy
func (r *FirstFitPolicy) SetAllocationDetails(profileName string, newStart, size int32, podUUID types.UID, nodename types.NodeName,
	allocationStatus inferencev1alpha1.AllocationStatus, discoveredGiprofile int32, Ciprofileid int32, Ciengprofileid int32,
	namespace string, podName string, gpuUuid string, resourceIdentifier types.UID) (*inferencev1alpha1.AllocationRequest, *inferencev1alpha1.AllocationResult) {
	
	return &inferencev1alpha1.AllocationRequest{
			Profile: profileName,
			PodRef: v1.ObjectReference{
				Kind:      "Pod",
				Namespace: namespace,
				Name:      podName,
				UID:       podUUID,
			},
		}, &inferencev1alpha1.AllocationResult{
			MigPlacement: inferencev1alpha1.Placement{
				Size:  size,
				Start: newStart,
			},
			GPUUUID:                     gpuUuid,
			Nodename:                    nodename,
			AllocationStatus:            allocationStatus,
			ConfigMapResourceIdentifier: resourceIdentifier,
			Conditions:                  []metav1.Condition{},
		}
}

// UpdateOrDeleteInstasliceAllocations updates or deletes allocations in an Instaslice object using generated clientset
func UpdateOrDeleteInstasliceAllocations(ctx context.Context, clientset *clientset.Clientset, instasliceName string, 
	allocResult *inferencev1alpha1.AllocationResult, allocRequest *inferencev1alpha1.AllocationRequest) error {
	
	// Get the current Instaslice object
	instaslice, err := clientset.OpenShiftOperatorV1alpha1().Instaslices("instaslice-operator").Get(ctx, instasliceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Instaslice object: %w", err)
	}

	// Initialize maps if nil
	if instaslice.Spec.PodAllocationRequests == nil {
		requests := make(map[types.UID]inferencev1alpha1.AllocationRequest)
		instaslice.Spec.PodAllocationRequests = &requests
	}
	if instaslice.Status.PodAllocationResults == nil {
		instaslice.Status.PodAllocationResults = make(map[string]inferencev1alpha1.AllocationResult)
	}

	if allocRequest != nil && allocResult != nil {
		// Update allocations
		(*instaslice.Spec.PodAllocationRequests)[allocRequest.PodRef.UID] = *allocRequest
		instaslice.Status.PodAllocationResults[string(allocRequest.PodRef.UID)] = *allocResult
	} else {
		// Delete operation - remove from both spec and status
		for uid := range *instaslice.Spec.PodAllocationRequests {
			uidStr := string(uid)
			if result, exists := instaslice.Status.PodAllocationResults[uidStr]; exists {
				if result.AllocationStatus.AllocationStatusDaemonset == string(inferencev1alpha1.AllocationStatusDeleted) {
					delete(*instaslice.Spec.PodAllocationRequests, uid)
					delete(instaslice.Status.PodAllocationResults, uidStr)
				}
			}
		}
	}

	// Update the object
	_, err = clientset.OpenShiftOperatorV1alpha1().Instaslices("instaslice-operator").Update(ctx, instaslice, metav1.UpdateOptions{})
	return err
}

// UpdateInstasliceAllocationWithClientset updates allocation using the generated clientset
func UpdateInstasliceAllocationWithClientset(ctx context.Context, clientset *clientset.Clientset, 
	instasliceName, namespace string, allocResult *inferencev1alpha1.AllocationResult, 
	allocRequest *inferencev1alpha1.AllocationRequest) error {
	
	// Get the current Instaslice object
	instaslice, err := clientset.OpenShiftOperatorV1alpha1().Instaslices(namespace).Get(ctx, instasliceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Instaslice object: %w", err)
	}

	// Initialize maps if nil
	if instaslice.Spec.PodAllocationRequests == nil {
		requests := make(map[types.UID]inferencev1alpha1.AllocationRequest)
		instaslice.Spec.PodAllocationRequests = &requests
	}
	if instaslice.Status.PodAllocationResults == nil {
		instaslice.Status.PodAllocationResults = make(map[string]inferencev1alpha1.AllocationResult)
	}

	if allocRequest != nil && allocResult != nil {
		// Update allocations
		(*instaslice.Spec.PodAllocationRequests)[allocRequest.PodRef.UID] = *allocRequest
		instaslice.Status.PodAllocationResults[string(allocRequest.PodRef.UID)] = *allocResult
	}

	// Update the object
	_, err = clientset.OpenShiftOperatorV1alpha1().Instaslices(namespace).Update(ctx, instaslice, metav1.UpdateOptions{})
	return err
}