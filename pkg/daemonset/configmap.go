package daemonset

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// createConfigMap creates a ConfigMap which is used by Pods to consume MIG device
func (c *DaemonsetController) createConfigMap(ctx context.Context, migGPUUUID string, namespace string, resourceIdentifier string) error {
	log := klog.FromContext(ctx)

	_, err := c.kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, resourceIdentifier, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		log.V(2).Info("ConfigMap not found, creating", "name", resourceIdentifier, "migGPUUUID", migGPUUUID)
		configMapToCreate := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceIdentifier,
				Namespace: namespace,
			},
			Data: map[string]string{
				"NVIDIA_VISIBLE_DEVICES": migGPUUUID,
				"CUDA_VISIBLE_DEVICES":   migGPUUUID,
			},
		}

		_, err := c.kubeClient.CoreV1().ConfigMaps(namespace).Create(ctx, configMapToCreate, metav1.CreateOptions{})
		if err != nil {
			log.Error(err, "failed to create ConfigMap", "name", resourceIdentifier)
			return err
		}

		log.V(2).Info("ConfigMap created successfully", "name", resourceIdentifier)
	}
	return nil
}

// deleteConfigMap manages lifecycle of configmap, deletes it once the pod is deleted from the system
func (c *DaemonsetController) deleteConfigMap(ctx context.Context, configMapName string, namespace string) error {
	log := klog.FromContext(ctx)

	err := c.kubeClient.CoreV1().ConfigMaps(namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.V(2).Info("configmap not found for deletion", "configmap", configMapName)
			return nil
		}
		return err
	}

	log.V(2).Info("ConfigMap deleted successfully", "name", configMapName)
	return nil
}

// checkConfigMapExists checks if a ConfigMap exists
func (c *DaemonsetController) checkConfigMapExists(ctx context.Context, name, namespace string) (bool, error) {
	log := klog.FromContext(ctx)

	_, err := c.kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.V(2).Info("ConfigMap not found", "name", name, "namespace", namespace)
			return false, nil
		}
		log.Error(err, "Error checking ConfigMap", "name", name, "namespace", namespace)
		return false, err
	}

	log.V(2).Info("ConfigMap exists", "name", name, "namespace", namespace)
	return true, nil
}
