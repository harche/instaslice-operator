package instaslice

import (
   "context"
   "fmt"
   "testing"

   inferencev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
   "github.com/openshift/instaslice-operator/pkg/operator/utils"
   v1 "k8s.io/api/core/v1"
   metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
   "k8s.io/apimachinery/pkg/types"
)

func TestNameToKey(t *testing.T) {
   c := &InstasliceController{}
   pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mypod"}}
   keys := c.nameToKey(pod)
   if len(keys) != 1 || keys[0] != "mypod" {
       t.Errorf("nameToKey returned %v, expected [\"mypod\"]", keys)
   }

   type badObj struct{}
   keys = c.nameToKey(&badObj{})
   if len(keys) != 0 {
       t.Errorf("nameToKey with bad object returned %v, expected []", keys)
   }
}

func TestExtractGpuProfile(t *testing.T) {
   c := &InstasliceController{}
   size, giProfile, ciProfile, ciEngProfile := c.extractGpuProfile(&inferencev1alpha1.Instaslice{}, "profile")
   if size != 0 || giProfile != 0 || ciProfile != 0 || ciEngProfile != 0 {
       t.Errorf("extractGpuProfile returned %d,%d,%d,%d for empty slice, expected 0,0,0,0", size, giProfile, ciProfile, ciEngProfile)
   }

   slice := &inferencev1alpha1.Instaslice{}
   slice.Status.NodeResources.MigPlacement = map[string]inferencev1alpha1.Mig{
       "gold": {
           Placements:     []inferencev1alpha1.Placement{{Size: 2, Start: 1}},
           GIProfileID:    100,
           CIProfileID:    200,
           CIEngProfileID: 300,
       },
   }
   size, giProfile, ciProfile, ciEngProfile = c.extractGpuProfile(slice, "gold")
   if size != 2 || giProfile != 100 || ciProfile != 200 || ciEngProfile != 300 {
       t.Errorf("extractGpuProfile returned %d,%d,%d,%d for slice, expected 2,100,200,300", size, giProfile, ciProfile, ciEngProfile)
   }
}

func TestIsPlacementAvailable(t *testing.T) {
   c := &InstasliceController{}
   slice := &inferencev1alpha1.Instaslice{}
   placement := inferencev1alpha1.Placement{Start: 1, Size: 2}
   if !c.isPlacementAvailable(placement, slice) {
       t.Errorf("isPlacementAvailable returned false, expected true for empty allocations")
   }

   slice.Status.PodAllocationResults = map[string]inferencev1alpha1.AllocationResult{
       "pod1": {MigPlacement: inferencev1alpha1.Placement{Start: 1, Size: 2}},
   }
   if c.isPlacementAvailable(placement, slice) {
       t.Errorf("isPlacementAvailable returned true, expected false for matching allocation")
   }
}

func TestFindNodeAndDeviceForSlice_Success(t *testing.T) {
   c := &InstasliceController{}
   profileName := "gold"
   placement := inferencev1alpha1.Placement{Start: 1, Size: 2}
   nodeName := "node1"
   uuid := "gpu-uuid"

   slice := &inferencev1alpha1.Instaslice{
       ObjectMeta: metav1.ObjectMeta{Name: nodeName},
       Status: inferencev1alpha1.InstasliceStatus{
           NodeResources: inferencev1alpha1.DiscoveredNodeResources{
               MigPlacement: map[string]inferencev1alpha1.Mig{
                   profileName: {
                       Placements:     []inferencev1alpha1.Placement{placement},
                       GIProfileID:    10,
                       CIProfileID:    20,
                       CIEngProfileID: 30,
                   },
               },
               NodeGPUs: []inferencev1alpha1.DiscoveredGPU{{GPUUUID: uuid}},
           },
       },
   }
   pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("pod-uid"), Name: "pod1", Namespace: "ns1"}}
   allocRequest, allocResult, err := c.findNodeAndDeviceForSlice(context.TODO(), slice, profileName, &utils.FirstFitPolicy{}, pod)
   if err != nil {
       t.Fatalf("findNodeAndDeviceForSlice returned error: %v", err)
   }
   if allocRequest.Profile != profileName {
       t.Errorf("request.Profile = %s, expected %s", allocRequest.Profile, profileName)
   }
   if allocRequest.PodRef.UID != pod.UID || allocRequest.PodRef.Name != pod.Name || allocRequest.PodRef.Namespace != pod.Namespace {
       t.Errorf("request.PodRef = %v, expected UID %s, Name %s, Namespace %s", allocRequest.PodRef, pod.UID, pod.Name, pod.Namespace)
   }
   expectedID := types.UID(fmt.Sprintf("%s-%s-%d", pod.Name, nodeName, placement.Start))
   if allocResult.ConfigMapResourceIdentifier != expectedID {
       t.Errorf("result.ConfigMapResourceIdentifier = %s, expected %s", allocResult.ConfigMapResourceIdentifier, expectedID)
   }
   if allocResult.MigPlacement.Start != placement.Start || allocResult.MigPlacement.Size != placement.Size {
       t.Errorf("result.MigPlacement = %+v, expected %+v", allocResult.MigPlacement, placement)
   }
   if allocResult.Nodename != types.NodeName(nodeName) {
       t.Errorf("result.Nodename = %s, expected %s", allocResult.Nodename, nodeName)
   }
}

func TestFindNodeAndDeviceForSlice_ProfileNotFound(t *testing.T) {
   c := &InstasliceController{}
   slice := &inferencev1alpha1.Instaslice{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
   slice.Status.NodeResources.MigPlacement = map[string]inferencev1alpha1.Mig{}
   pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("pod"), Name: "pod", Namespace: "ns"}}
   _, _, err := c.findNodeAndDeviceForSlice(context.TODO(), slice, "missing", &utils.FirstFitPolicy{}, pod)
   expected := fmt.Sprintf("profile %s not found on node %s", "missing", slice.Name)
   if err == nil || err.Error() != expected {
       t.Errorf("expected error %q, got %v", expected, err)
   }
}

func TestFindNodeAndDeviceForSlice_NoAvailablePlacement(t *testing.T) {
   c := &InstasliceController{}
   profileName := "gold"
   slice := &inferencev1alpha1.Instaslice{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
   slice.Status.NodeResources.MigPlacement = map[string]inferencev1alpha1.Mig{
       profileName: {Placements: []inferencev1alpha1.Placement{}},
   }
   pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: types.UID("pod"), Name: "pod", Namespace: "ns"}}
   _, _, err := c.findNodeAndDeviceForSlice(context.TODO(), slice, profileName, &utils.FirstFitPolicy{}, pod)
   expected := fmt.Sprintf("no available placement found for profile %s on node %s", profileName, slice.Name)
   if err == nil || err.Error() != expected {
       t.Errorf("expected error %q, got %v", expected, err)
   }
}