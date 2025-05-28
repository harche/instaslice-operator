package utils

import (
	"fmt"
	"strings"

	"github.com/openshift/instaslice-operator/pkg/operator/constants"
	v1 "k8s.io/api/core/v1"
)

// TransformResources transforms nvidia.com/mig-* resources to instaslice.redhat.com/mig-*
func TransformResources(resources *v1.ResourceRequirements) {
	transformResourceList := func(resourceLists ...*v1.ResourceList) {
		for _, resourceList := range resourceLists {
			if *resourceList == nil {
				*resourceList = make(v1.ResourceList)
			}
			for resourceName, quantity := range *resourceList {
				if strings.HasPrefix(string(resourceName), constants.NvidiaMIGPrefix) {
					newResourceName := strings.Replace(string(resourceName), constants.NvidiaMIGPrefix, 
						fmt.Sprintf("%smig-", constants.OrgInstaslicePrefix), 1)
					delete(*resourceList, resourceName)
					(*resourceList)[v1.ResourceName(newResourceName)] = quantity
				}
			}
		}
	}
	transformResourceList(&resources.Limits, &resources.Requests)
}