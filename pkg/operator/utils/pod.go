package utils

import (
	"regexp"
	"strings"

	"github.com/openshift/instaslice-operator/pkg/operator/constants"
	v1 "k8s.io/api/core/v1"
)

// ExtractProfileName extracts the MIG profile name from container resource limits
func ExtractProfileName(limits v1.ResourceList) string {
	profileName := ""
	for k := range limits {
		if strings.Contains(k.String(), "mig-") {
			re := regexp.MustCompile(`(\d+g\.\d+gb)`)
			match := re.FindStringSubmatch(k.String())
			if len(match) > 1 {
				profileName = match[1]
			}
		}
	}
	return profileName
}

// CheckIfPodGatedByInstaSlice checks if a pod has InstaSlice scheduling gate
func CheckIfPodGatedByInstaSlice(pod *v1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, gate := range pod.Spec.SchedulingGates {
		if gate.Name == constants.GateName && pod.Status.Phase == v1.PodPending {
			for _, cond := range pod.Status.Conditions {
				if strings.Contains(cond.Message, "blocked") {
					return true
				}
			}
		}
	}
	return false
}

// IsPodGatedByOthers checks if pod has scheduling gates other than InstaSlice gate
func IsPodGatedByOthers(pod *v1.Pod) bool {
	for _, gate := range pod.Spec.SchedulingGates {
		if gate.Name != constants.GateName {
			return true
		}
	}
	return false
}

// UnGatePod removes InstaSlice scheduling gate from pod
func UnGatePod(pod *v1.Pod) *v1.Pod {
	for i, gate := range pod.Spec.SchedulingGates {
		if gate.Name == constants.GateName {
			pod.Spec.SchedulingGates = append(pod.Spec.SchedulingGates[:i], pod.Spec.SchedulingGates[i+1:]...)
			break
		}
	}
	return pod
}

// HasMIGResource checks if a pod has resource requests or limits with nvidia.com/mig-* pattern
func HasMIGResource(pod *v1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		// Check resource limits
		for resourceName := range container.Resources.Limits {
			if strings.HasPrefix(string(resourceName), constants.NvidiaMIGPrefix) {
				return true
			}
		}
		// Check resource requests
		for resourceName := range container.Resources.Requests {
			if strings.HasPrefix(string(resourceName), constants.NvidiaMIGPrefix) {
				return true
			}
		}
	}
	return false
}

// ContainsFinalizer checks if a pod contains a specific finalizer
func ContainsFinalizer(pod *v1.Pod, finalizer string) bool {
	for _, f := range pod.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// RemoveFinalizer removes a finalizer from a pod and returns true if it was removed
func RemoveFinalizer(pod *v1.Pod, finalizer string) bool {
	for i, f := range pod.Finalizers {
		if f == finalizer {
			pod.Finalizers = append(pod.Finalizers[:i], pod.Finalizers[i+1:]...)
			return true
		}
	}
	return false
}