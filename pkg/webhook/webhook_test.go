package webhook

import (
   "encoding/json"
   "fmt"
   "testing"

   jsonpatch "github.com/evanphx/json-patch"
   . "github.com/onsi/gomega"
   admissionv1 "k8s.io/api/admission/v1"
   corev1 "k8s.io/api/core/v1"
   "k8s.io/apimachinery/pkg/api/resource"
   "k8s.io/apimachinery/pkg/runtime"
   admissionctl "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// createRL builds a ResourceList from a map[string]string
func createRL(m map[string]string) corev1.ResourceList {
   rl := corev1.ResourceList{}
   for k, v := range m {
       rl[corev1.ResourceName(k)] = resource.MustParse(v)
   }
   return rl
}

// TestAuthorized verifies that the webhook patches Pods with NVIDIA MIG resources
// and leaves other Pods unmodified.
func TestAuthorized(t *testing.T) {
   hook := NewWebhook()

   tests := []struct {
       name          string
       limits        map[string]string
       expectPatch   bool
       expectedQuota string
   }{
       {name: "no-mig", limits: map[string]string{"cpu": "100m"}, expectPatch: false},
       {name: "with-mig", limits: map[string]string{"nvidia.com/mig-1g.5gb": "1"}, expectPatch: true, expectedQuota: "5Gi"},
   }

   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           g := NewWithT(t)
           // build Pod
           pod := &corev1.Pod{
               Spec: corev1.PodSpec{Containers: []corev1.Container{{
                   Resources: corev1.ResourceRequirements{Limits: createRL(tt.limits)},
               }}},
           }
           raw, err := json.Marshal(pod)
           g.Expect(err).NotTo(HaveOccurred())
           req := admissionctl.Request{
               AdmissionRequest: admissionv1.AdmissionRequest{
                   Object: runtime.RawExtension{Raw: raw},
               },
           }
           resp := hook.Authorized(req)

           if tt.expectPatch {
               g.Expect(resp.Patches).NotTo(BeEmpty(), "expected patches but found none")
               // apply patches
               patchBytes, err := json.Marshal(resp.Patches)
               g.Expect(err).NotTo(HaveOccurred())
               patchObj, err := jsonpatch.DecodePatch(patchBytes)
               g.Expect(err).NotTo(HaveOccurred())
               origBytes, err := json.Marshal(pod)
               g.Expect(err).NotTo(HaveOccurred())
               patchedBytes, err := patchObj.Apply(origBytes)
               g.Expect(err).NotTo(HaveOccurred())
               var modified corev1.Pod
               g.Expect(json.Unmarshal(patchedBytes, &modified)).To(Succeed())
               q, found := modified.Spec.Containers[0].Resources.Limits[corev1.ResourceName(QuotaResourceName)]
               g.Expect(found).To(BeTrue(), "quota resource not found")
               g.Expect(q.Cmp(resource.MustParse(tt.expectedQuota))).To(Equal(0), "unexpected quota value")
               // Verify instaslice pod label is added
               val, ok := modified.Labels["instaslice.com/pod"]
               g.Expect(ok).To(BeTrue(), "expected instaslice.com/pod label to be set")
               g.Expect(val).To(Equal("true"), "unexpected instaslice.com/pod label value")
           } else {
               g.Expect(resp.Patches).To(BeEmpty(), "expected no patches but found some")
           }
       })
   }
}

// TestTransformResources ensures that only resources with the NVIDIA prefix are transformed
func TestTransformResources(t *testing.T) {
   createRL := func(m map[string]string) corev1.ResourceList {
       rl := corev1.ResourceList{}
       for k, v := range m {
           rl[corev1.ResourceName(k)] = resource.MustParse(v)
       }
       return rl
   }

   tests := []struct {
       name             string
       limits           map[string]string
       requests         map[string]string
       expectedLimits   map[string]string
       expectedRequests map[string]string
   }{
       {
           name: "Transform valid resources",
           limits: map[string]string{"nvidia.com/mig-1g": "1", "nvidia.com/mig-2g": "2"},
           requests: map[string]string{"nvidia.com/mig-1g": "1"},
           expectedLimits: map[string]string{"instaslice.redhat.com/mig-1g": "1", "instaslice.redhat.com/mig-2g": "2"},
           expectedRequests: map[string]string{"instaslice.redhat.com/mig-1g": "1"},
       },
       {
           name: "Do not transform unrelated resources",
           limits: map[string]string{"other.com/mig-1g": "3"},
           requests: map[string]string{"unrelated.com/mig-1g": "4"},
           expectedLimits: map[string]string{"other.com/mig-1g": "3"},
           expectedRequests: map[string]string{"unrelated.com/mig-1g": "4"},
       },
   }

   hook := NewWebhook()
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           g := NewWithT(t)
           resources := &corev1.ResourceRequirements{
               Limits:   createRL(tt.limits),
               Requests: createRL(tt.requests),
           }
           // invoke transformation
           hook.transformResources(resources)

           // compare lists
           cmp := func(actual corev1.ResourceList, expected map[string]string) {
               g.Expect(len(actual)).To(Equal(len(expected)))
               for k, exp := range expected {
                   q, found := actual[corev1.ResourceName(k)]
                   g.Expect(found).To(BeTrue(), fmt.Sprintf("expected %s", k))
                   g.Expect(q.Cmp(resource.MustParse(exp))).To(Equal(0), fmt.Sprintf("%s quantity mismatch", k))
               }
           }
           cmp(resources.Limits, tt.expectedLimits)
           cmp(resources.Requests, tt.expectedRequests)
       })
   }
}