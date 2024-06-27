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

package controller

import (
	"context"
	"regexp"
	"strings"

	inferencev1 "codeflare.dev/instaslice/api/v1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// InstasliceReconciler reconciles a Instaslice object
type InstasliceReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	kubeClient *kubernetes.Clientset
}

// AllocationPolicy interface with a single method
type AllocationPolicy interface {
	SetAllocationDetails(profileName string, newStart, size uint32, podUUID string, nodename string, processed string, discoveredGiprofile int, Ciprofileid int, Ciengprofileid int, namespace string, podName string) *inferencev1.AllocationDetails
}

type RightToLeftPolicy struct{}

type LeftToRightPolicy struct{}

type FirstFitPolicy struct{}

//+kubebuilder:rbac:groups=inference.codeflare.dev,resources=instaslices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=inference.codeflare.dev,resources=instaslices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=inference.codeflare.dev,resources=instaslices/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Instaslice object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile

func (r *InstasliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	logger := log.Log.WithName("controller-name")
	var policy AllocationPolicy
	policy = &FirstFitPolicy{}
	pod := &v1.Pod{}
	//var podName string
	var isPodGated = false
	err := r.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			// Pod not found. It might have been deleted.
			return ctrl.Result{}, nil
		}
		// Error fetching the Pod
		return ctrl.Result{}, err
	}

	isPodGated = checkIfPodGated(pod, isPodGated)

	if pod.Labels != nil && pod.Labels["processedbydeamonset"] == "true" {
		r.unGatePod(context.TODO(), pod.Name, req, pod, logger)
	}

	if pod.Labels != nil && pod.Labels["processedbycontroller"] == "true" {
		return ctrl.Result{}, err
	}

	if isPodGated {
		//Assume pod only has one container with one GPU requests
		limits := pod.Spec.Containers[0].Resources.Limits
		profileName := r.extractProfileName(limits)
		logger.Info("The profile name obtained", "name", profileName)
		// List all instances
		var instasliceList inferencev1.InstasliceList

		if err := r.List(ctx, &instasliceList, &client.ListOptions{}); err != nil {
			logger.Error(err, "Error listing Instaslice")
		}

		for _, instaslice := range instasliceList.Items {
			//TODO:Avoid empty GPU string
			var gpuUUID = ""
			gpuUUID, reportError, result, errSelectingDevice := r.findDeviceForASlice(ctx, instaslice, gpuUUID, profileName, policy, pod, logger)
			if reportError {
				return result, errSelectingDevice
			}
			r.addLabelsToCreateSlice(pod, instaslice.Name, gpuUUID)
			// Retry update operation with backoff
			retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				return r.Client.Update(ctx, pod)
			})
			if retryErr != nil {
				return ctrl.Result{}, retryErr
			}
			logger.Info("Done adding label to the pod")
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *InstasliceReconciler) findDeviceForASlice(ctx context.Context, instaslice inferencev1.Instaslice, gpuUUID string, profileName string, policy AllocationPolicy, pod *v1.Pod, logger logr.Logger) (string, bool, reconcile.Result, error) {
	//TODO: discover this value, this may work for A100 and H100 for now.
	largestIndex := uint32(7)
	for gpuuuid, _ := range instaslice.Spec.MigGPUUUID {
		gpuUUID = gpuuuid
		if instaslice.Spec.Allocations == nil {
			instaslice.Spec.Allocations = make(map[string]inferencev1.AllocationDetails)
		}
		maxStart := r.extractMaxStart(instaslice, gpuUUID)
		size, discoveredGiprofile, Ciprofileid, Ciengprofileid := r.extractGpuProfile(instaslice, profileName)
		if maxStart+uint32(size) <= largestIndex {
			logger.Info("Device where the slice will be placed", "DeviceUUID", gpuUUID)
		}

		newStart := maxStart - uint32(size)
		logger.Info("The placement is ", "index", newStart)
		allocDetails := policy.SetAllocationDetails(profileName, uint32(newStart), uint32(size),
			string(pod.UID), instaslice.Name, "no", discoveredGiprofile,
			Ciprofileid, Ciengprofileid, pod.Namespace, pod.Name)
		instaslice.Spec.Allocations[gpuUUID] = *allocDetails
		if err := r.Update(ctx, &instaslice); err != nil {
			logger.Error(err, "Error updating instaslice allocations")
			return "", true, ctrl.Result{}, err
		}
		return gpuUUID, false, reconcile.Result{}, nil
	}
	return gpuUUID, false, reconcile.Result{}, nil
}

// Extract profile name from the container limits spec
func (*InstasliceReconciler) extractProfileName(limits v1.ResourceList) string {
	profileName := ""
	for k, _ := range limits {
		if strings.Contains(k.String(), "nvidia") {

			re := regexp.MustCompile(`(\d+g\.\d+gb)`)
			match := re.FindStringSubmatch(k.String())
			if len(match) > 1 {
				profileName = match[1]
			} else {
				log.Log.Info("No match found")
			}
		}
	}
	return profileName
}

// Extract NVML specific attributes for GPUs, this will change for different generations of the GPU.
func (*InstasliceReconciler) extractGpuProfile(instaslice inferencev1.Instaslice, profileName string) (int, int, int, int) {
	var size int
	var discoveredGiprofile int
	var Ciprofileid int
	var Ciengprofileid int
	for _, item := range instaslice.Spec.Migplacement {
		if item.Profile == profileName {
			for _, aPlacement := range item.Placements {
				size = aPlacement.Size
				discoveredGiprofile = item.Giprofileid
				Ciprofileid = item.CIProfileID
				Ciengprofileid = item.CIEngProfileID
				break
			}
		}
	}
	return size, discoveredGiprofile, Ciprofileid, Ciengprofileid
}

// Walk through all the allocated devices and get the max position where the slice could be allocated.
// the implementation is specific to first fit and this is needed until we get new strategy implemented
// in GPU operator.
func (*InstasliceReconciler) extractMaxStart(instaslice inferencev1.Instaslice, gpuUUID string) uint32 {
	var maxSize uint32 = 0
	var maxStart uint32 = 0
	for _, item := range instaslice.Spec.Prepared {
		if item.Parent == gpuUUID {
			if maxSize < item.Size {
				maxSize = item.Size
			}
			if maxStart < item.Start {
				maxStart = item.Start
			}
		}
	}
	return maxStart
}

// Since we dont have user facing CRD, we make our way with attaching labels to the pods to indicate processing status.
func (*InstasliceReconciler) addLabelsToCreateSlice(pod *v1.Pod, nodeName string, gpuUuid string) {
	labelMap := make(map[string]string)
	labelMap["generateslice"] = "true"
	labelMap["processedbycontroller"] = "true"
	labelMap["nodeName"] = nodeName
	labelMap["gpuUUID"] = gpuUuid
	pod.SetLabels(labelMap)
}

func checkIfPodGated(pod *v1.Pod, isPodGated bool) bool {
	for _, gate := range pod.Spec.SchedulingGates {
		if gate.Name == "org.instaslice/accelarator" {
			if pod.Status.Phase == v1.PodPending && strings.Contains(pod.Status.Conditions[0].Message, "blocked") {
				isPodGated = true
			}
		}
	}
	return isPodGated
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstasliceReconciler) SetupWithManager(mgr ctrl.Manager) error {

	restConfig := mgr.GetConfig()

	var err error
	r.kubeClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).Named("InstaSlice").
		Complete(r)
}

func (r *InstasliceReconciler) unGatePod(ctx context.Context, podName string, req reconcile.Request, podUpdate *v1.Pod, logger logr.Logger) {
	err := r.Client.Get(ctx, client.ObjectKey{Name: podName, Namespace: req.Namespace}, podUpdate)
	if err != nil {
		//TODO: handle error condition
		logger.Error(err, "Failed to obtain pod from API server")
	}
	for i, gate := range podUpdate.Spec.SchedulingGates {
		if gate.Name == "org.instaslice/accelarator" {
			podUpdate.Spec.SchedulingGates = append(podUpdate.Spec.SchedulingGates[:i], podUpdate.Spec.SchedulingGates[i+1:]...)
		}
	}
	errUngating := r.Update(ctx, podUpdate)
	if errUngating != nil {
		logger.Error(errUngating, "Failed to ungate the pod")
	}
}

// Policy based allocation - FirstFit
func (r *FirstFitPolicy) SetAllocationDetails(profileName string, newStart, size uint32, podUUID, nodename string,
	processed string, discoveredGiprofile int, Ciprofileid int, Ciengprofileid int,
	namespace string, podName string) *inferencev1.AllocationDetails {
	return &inferencev1.AllocationDetails{
		Profile:        profileName,
		Start:          uint32(newStart),
		Size:           uint32(size),
		PodUUID:        podUUID,
		Nodename:       nodename,
		Processed:      processed,
		Giprofileid:    discoveredGiprofile,
		CIProfileID:    Ciprofileid,
		CIEngProfileID: Ciengprofileid,
		Namespace:      namespace,
		PodName:        podName,
	}
}

// Policy based allocation - LeftToRIght
func (l *LeftToRightPolicy) SetAllocationDetails(profileName string, newStart, size uint32, podUUID, nodename string,
	processed string, discoveredGiprofile int, Ciprofileid int, Ciengprofileid int,
	namespace string, podName string) *inferencev1.AllocationDetails {
	// Implement the left-to-right policy here
	return &inferencev1.AllocationDetails{}
}

// Policy based allocation - RigghToLeft
func (l *RightToLeftPolicy) SetAllocationDetails(profileName string, newStart, size uint32, podUUID, nodename string,
	processed string, discoveredGiprofile int, Ciprofileid int, Ciengprofileid int,
	namespace string, podName string) *inferencev1.AllocationDetails {
	// Implement the left-to-right policy here
	return &inferencev1.AllocationDetails{}
}
