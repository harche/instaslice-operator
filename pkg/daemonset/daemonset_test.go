package daemonset

import (
   "context"
   "testing"

   "github.com/NVIDIA/go-nvml/pkg/nvml"
   inferencev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
   "github.com/openshift/instaslice-operator/pkg/operator/constants"
   corev1 "k8s.io/api/core/v1"
   "k8s.io/apimachinery/pkg/api/errors"
   "k8s.io/apimachinery/pkg/api/resource"
   metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
   "k8s.io/client-go/kubernetes/fake"
)

// Tests for configmap operations
func TestCreateConfigMap(t *testing.T) {
   client := fake.NewSimpleClientset()
   ctrl := &DaemonsetController{kubeClient: client}
   ctx := context.Background()
   name := "test-cm"
   ns := "test-ns"
   uuid := "abc-123"
   if err := ctrl.createConfigMap(ctx, uuid, ns, name); err != nil {
       t.Fatalf("createConfigMap returned error: %v", err)
   }
   cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
   if err != nil {
       t.Fatalf("expected ConfigMap to be created, got error: %v", err)
   }
   if cm.Data["NVIDIA_VISIBLE_DEVICES"] != uuid {
       t.Errorf("unexpected NVIDIA_VISIBLE_DEVICES: got %v, want %v", cm.Data["NVIDIA_VISIBLE_DEVICES"], uuid)
   }
   if cm.Data["CUDA_VISIBLE_DEVICES"] != uuid {
       t.Errorf("unexpected CUDA_VISIBLE_DEVICES: got %v, want %v", cm.Data["CUDA_VISIBLE_DEVICES"], uuid)
   }
   // Calling again should not error
   if err := ctrl.createConfigMap(ctx, uuid, ns, name); err != nil {
       t.Errorf("createConfigMap on existing ConfigMap returned error: %v", err)
   }
}

func TestCheckConfigMapExists(t *testing.T) {
   client := fake.NewSimpleClientset()
   ctrl := &DaemonsetController{kubeClient: client}
   ctx := context.Background()
   name := "test-cm"
   ns := "test-ns"
   exists, err := ctrl.checkConfigMapExists(ctx, name, ns)
   if err != nil {
       t.Fatalf("checkConfigMapExists returned error: %v", err)
   }
   if exists {
       t.Errorf("checkConfigMapExists: expected false, got true")
   }
   // create ConfigMap and test again
   if _, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
       ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
       Data:       map[string]string{},
   }, metav1.CreateOptions{}); err != nil {
       t.Fatalf("failed to create ConfigMap for test: %v", err)
   }
   exists, err = ctrl.checkConfigMapExists(ctx, name, ns)
   if err != nil {
       t.Fatalf("checkConfigMapExists returned error: %v", err)
   }
   if !exists {
       t.Errorf("checkConfigMapExists: expected true, got false")
   }
}

func TestDeleteConfigMap(t *testing.T) {
   client := fake.NewSimpleClientset()
   ctrl := &DaemonsetController{kubeClient: client}
   ctx := context.Background()
   name := "test-cm"
   ns := "test-ns"
   // Delete non-existent should not error
   if err := ctrl.deleteConfigMap(ctx, name, ns); err != nil {
       t.Errorf("deleteConfigMap non-existent returned error: %v", err)
   }
   // create ConfigMap for deletion
   if _, err := client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
       ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
       Data:       map[string]string{},
   }, metav1.CreateOptions{}); err != nil {
       t.Fatalf("failed to create ConfigMap for delete test: %v", err)
   }
   if err := ctrl.deleteConfigMap(ctx, name, ns); err != nil {
       t.Errorf("deleteConfigMap existing returned error: %v", err)
   }
   // verify deletion
   if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{}); !errors.IsNotFound(err) {
       t.Errorf("expected ConfigMap to be deleted, got error: %v", err)
   }
}

// Tests for MIG profile utilities
func TestGetMigMemorySizeInGB(t *testing.T) {
   oneGB := uint64(1024 * 1024 * 1024)
   if got := getMigMemorySizeInGB(oneGB, 1024); got != 1 {
       t.Errorf("expected 1GB, got %d", got)
   }
   // half GB should round: total=1GB, mig=512MB => 0.5*1=0.5 -> round to 1
   if got := getMigMemorySizeInGB(oneGB, 512); got != 1 {
       t.Errorf("expected 1GB for half fraction, got %d", got)
   }
   // quarter GB: 256MB => 0.25*1=0.25 -> round to 0
   if got := getMigMemorySizeInGB(oneGB, 256); got != 0 {
       t.Errorf("expected 0GB for quarter fraction, got %d", got)
   }
}

func TestNewMigProfile(t *testing.T) {
   giID, ciID, ciEngID := 1, 2, 3
   giSlices, ciSlices := uint32(4), uint32(5)
   memMB, totalMem := uint64(100), uint64(200*1024*1024)
   mp := NewMigProfile(giID, ciID, ciEngID, giSlices, ciSlices, memMB, totalMem)
   if mp.GIProfileID != giID || mp.CIProfileID != ciID || mp.CIEngProfileID != ciEngID {
       t.Errorf("unexpected profile IDs: got GI %d CI %d CIEng %d", mp.GIProfileID, mp.CIProfileID, mp.CIEngProfileID)
   }
   if mp.G != int(giSlices) || mp.C != int(ciSlices) {
       t.Errorf("unexpected slice counts: got G %d C %d", mp.G, mp.C)
   }
   // GB field should match getMigMemorySizeInGB
   expectedGB := int(getMigMemorySizeInGB(totalMem, memMB))
   if mp.GB != expectedGB {
       t.Errorf("unexpected GB: got %d, want %d", mp.GB, expectedGB)
   }
}

func TestMigProfileStringAndAttributes(t *testing.T) {
   // Case C == G, no attributes
   mp := MigProfile{C: 2, G: 2, GB: 3, GIProfileID: 0}
   if attrs := mp.Attributes(); len(attrs) != 0 {
       t.Errorf("expected no attributes, got %v", attrs)
   }
   if s := mp.String(); s != "2g.3gb" {
       t.Errorf("unexpected String(): got %s", s)
   }
   // Case C == G, with media extension attribute
   mp2 := MigProfile{C: 1, G: 1, GB: 2, GIProfileID: nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1}
   // For this GIProfileID, Attributes returns ["me"]
   attrs := mp2.Attributes()
   if len(attrs) != 1 || attrs[0] != constants.AttributeMediaExtensions {
       t.Errorf("expected attribute %s, got %v", constants.AttributeMediaExtensions, attrs)
   }
   // Suffix "+me"
   if s2 := mp2.String(); s2 != "1g.2gb+me" {
       t.Errorf("unexpected String() with attribute: got %s", s2)
   }
}

func TestCalculateTotalMemoryGB(t *testing.T) {
   gpus := []inferencev1alpha1.DiscoveredGPU{
       {GPUMemory: resource.MustParse("1Gi")},
       {GPUMemory: resource.MustParse("2Gi")},
   }
   total, err := CalculateTotalMemoryGB(gpus)
   if err != nil {
       t.Fatalf("CalculateTotalMemoryGB returned error: %v", err)
   }
   if total != 3 {
       t.Errorf("expected total memory 3GB, got %v", total)
   }
   // empty slice should return 0
   total2, err2 := CalculateTotalMemoryGB(nil)
   if err2 != nil {
       t.Fatalf("CalculateTotalMemoryGB returned error: %v", err2)
   }
   if total2 != 0 {
       t.Errorf("expected total memory 0GB, got %v", total2)
   }
}