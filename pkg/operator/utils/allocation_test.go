package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
	clientsetfake "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned/fake"
)

// TestUpdateOrDeleteInstasliceAllocations_DeleteAllocation verifies that allocations
// with a Deleted status are removed from both spec and status maps.
func TestUpdateOrDeleteInstasliceAllocations_DeleteAllocation(t *testing.T) {
	ctx := context.TODO()
	// Prepare a fake Instaslice with one allocation marked as Deleted
	uid := types.UID("test-pod-uid")
	instaslice := &v1alpha1.Instaslice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-insta",
			Namespace: "instaslice-operator",
		},
		Spec: v1alpha1.InstasliceSpec{
			PodAllocationRequests: &map[types.UID]v1alpha1.AllocationRequest{
				uid: {
					Profile: "test-profile",
					PodRef: corev1.ObjectReference{
						Namespace: "instaslice-operator",
						Name:      "test-pod",
						UID:       uid,
					},
				},
			},
		},
		Status: v1alpha1.InstasliceStatus{
			PodAllocationResults: map[string]v1alpha1.AllocationResult{
				string(uid): {
					AllocationStatus: v1alpha1.AllocationStatus{
						AllocationStatusDaemonset: string(v1alpha1.AllocationStatusDeleted),
					},
				},
			},
		},
	}
	// Initialize fake clientset with the Instaslice object
	client := clientsetfake.NewSimpleClientset(instaslice)

	// Invoke deletion logic (allocRequest and allocResult nil triggers delete branch)
	err := UpdateOrDeleteInstasliceAllocations(ctx, client, instaslice.Name, nil, nil)
	assert.NoError(t, err)

	// Retrieve the updated Instaslice and verify removal of allocations
	updated, err := client.OpenShiftOperatorV1alpha1().Instaslices(instaslice.Namespace).
		Get(ctx, instaslice.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	// Both spec and status maps should be empty after deletion
	assert.Empty(t, *updated.Spec.PodAllocationRequests)
	assert.Empty(t, updated.Status.PodAllocationResults)
}
