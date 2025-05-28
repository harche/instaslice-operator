package daemonset

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	inferencev1alpha1 "github.com/openshift/instaslice-operator/pkg/apis/instasliceoperator/v1alpha1"
	"github.com/openshift/instaslice-operator/pkg/operator/constants"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

// createCiAndGiProfiles creates compute and GPU instances for a pod allocation
func (c *DaemonsetController) createCiAndGiProfiles(ctx context.Context, instaslice *inferencev1alpha1.Instaslice, podUID types.UID) error {
	log := klog.FromContext(ctx)
	podRef := (*instaslice.Spec.PodAllocationRequests)[podUID].PodRef
	allocResult := instaslice.Status.PodAllocationResults[string(podUID)]

	log.V(2).Info("creating allocation for pod", "podRef", podRef.Name)

	allocationRequest, haveReq := (*instaslice.Spec.PodAllocationRequests)[podUID]
	if !haveReq {
		log.V(2).Info("No matching PodAllocationRequest for this result; skipping")
		return nil
	}

	device, retCode := nvml.DeviceGetHandleByUUID(allocResult.GPUUUID)
	if retCode != nvml.SUCCESS {
		return fmt.Errorf("error fetching GPU device handle, GPUUUID: %s, error: %v", allocResult.GPUUUID, retCode)
	}

	selectedMig, ok := instaslice.Status.NodeResources.MigPlacement[allocationRequest.Profile]
	if !ok {
		return fmt.Errorf("requested MIG profile not found on the node, node: %s profile: %s", instaslice.Name, allocationRequest.Profile)
	}

	placement := nvml.GpuInstancePlacement{
		Start: uint32(allocResult.MigPlacement.Start),
		Size:  uint32(allocResult.MigPlacement.Size),
	}

	giProfileInfo, retGI := device.GetGpuInstanceProfileInfo(int(selectedMig.GIProfileID))
	if retGI != nvml.SUCCESS {
		return fmt.Errorf("cannot get GI profile info, GIProfileID: %d, error: %v", selectedMig.GIProfileID, retGI)
	}

	createdMigInfos, err := c.createSliceAndPopulateMigInfos(
		ctx, device, giProfileInfo, placement, selectedMig.CIProfileID, podRef.Name)
	if err != nil {
		return fmt.Errorf("MIG creation not successful: %w", err)
	}

	for migUuid, migDevice := range createdMigInfos {
		if migDevice.start == allocResult.MigPlacement.Start &&
			migDevice.uuid == allocResult.GPUUUID &&
			giProfileInfo.Id == migDevice.giInfo.ProfileId {

			exists, _ := c.checkConfigMapExists(ctx, string(allocResult.ConfigMapResourceIdentifier), podRef.Namespace)
			if exists {
				log.V(2).Info("Skipping updating pod", "podRef", podRef.Name)
				continue
			} else if err := c.createConfigMap(ctx, migUuid, podRef.Namespace, string(allocResult.ConfigMapResourceIdentifier)); err != nil {
				return err
			}
			log.V(2).Info("done creating mig slice", "pod", podRef.Name, "parentgpu", allocResult.GPUUUID, "miguuid", migUuid)
			break
		}
	}
	return nil
}

func (c *DaemonsetController) cleanUpCiAndGi(ctx context.Context, allocationResult *inferencev1alpha1.AllocationResult, podRef corev1.ObjectReference) error {
	log := klog.FromContext(ctx)

	parent, ret := nvml.DeviceGetHandleByUUID(allocationResult.GPUUUID)
	if ret != nvml.SUCCESS {
		log.Error(nil, "error obtaining GPU handle for cleanup", "error", ret)
		return fmt.Errorf("unable to get device handle: %v", ret)
	}

	migInfos, err := populateMigDeviceInfos(parent)
	if err != nil {
		return fmt.Errorf("unable to walk MIGs: %v", err)
	}

	for miguuid, migdevice := range migInfos {
		if migdevice.uuid == allocationResult.GPUUUID &&
			migdevice.start == allocationResult.MigPlacement.Start &&
			migdevice.size == allocationResult.MigPlacement.Size {

			gi, ret := parent.GetGpuInstanceById(int(migdevice.giInfo.Id))
			if ret != nvml.SUCCESS {
				return fmt.Errorf("unable to find GI: %v", ret)
			}

			ci, ret := gi.GetComputeInstanceById(int(migdevice.ciInfo.Id))
			if ret != nvml.SUCCESS {
				return fmt.Errorf("unable to find CI: %v", ret)
			}

			// Destroy CI
			ret = ci.Destroy()
			if ret != nvml.SUCCESS {
				return fmt.Errorf("unable to destroy CI: %v", ret)
			}

			// Destroy GI
			ret = gi.Destroy()
			if ret != nvml.SUCCESS {
				return fmt.Errorf("unable to destroy GI: %v", ret)
			}

			log.V(2).Info("Successfully destroyed MIG resources", "allocationResult", allocationResult, "podRef", podRef, "MIGuuid", miguuid)
			return nil
		}
	}
	return nil
}

// createSliceAndPopulateMigInfos creates GPU and compute instances and returns MIG device info
func (c *DaemonsetController) createSliceAndPopulateMigInfos(ctx context.Context, device nvml.Device,
	giProfileInfo nvml.GpuInstanceProfileInfo, placement nvml.GpuInstancePlacement,
	ciProfileId int32, podName string) (map[string]*MigDeviceInfo, error) {

	log := klog.FromContext(ctx)
	log.V(2).Info("creating slice for pod", "pod", podName)

	var gi nvml.GpuInstance
	var ret nvml.Return

	gi, ret = device.CreateGpuInstanceWithPlacement(&giProfileInfo, &placement)
	if ret != nvml.SUCCESS {
		switch ret {
		case nvml.ERROR_INSUFFICIENT_RESOURCES:
			// Handle case where GPU instance already exists
			gpuInstances, ret := device.GetGpuInstances(&giProfileInfo)
			if ret != nvml.SUCCESS {
				return nil, fmt.Errorf("gpu instances cannot be listed: %v", ret)
			}

			for _, gpuInstance := range gpuInstances {
				gpuInstanceInfo, ret := gpuInstance.GetInfo()
				if ret != nvml.SUCCESS {
					return nil, fmt.Errorf("unable to obtain gpu instance info: %v", ret)
				}

				parentUuid, ret := gpuInstanceInfo.Device.GetUUID()
				if ret != nvml.SUCCESS {
					return nil, fmt.Errorf("unable to obtain parent gpu uuid: %v", ret)
				}

				gpuUUID, ret := device.GetUUID()
				if ret != nvml.SUCCESS {
					log.Error(nil, "unable to obtain parent gpu uuid", "error", ret)
				}

				if gpuInstanceInfo.Placement.Start == placement.Start && parentUuid == gpuUUID {
					gi, ret = device.GetGpuInstanceById(int(gpuInstanceInfo.Id))
					if ret != nvml.SUCCESS {
						return nil, fmt.Errorf("unable to obtain gi post iteration: %v", ret)
					}
					break
				}
			}
		default:
			return nil, fmt.Errorf("gpu instance creation failed: %v", ret)
		}
	}

	ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(int(ciProfileId), 0)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting compute instance profile info: %v", ret)
	}

	ci, ret := gi.CreateComputeInstance(&ciProfileInfo)
	if ret != nvml.SUCCESS && ret != nvml.ERROR_INSUFFICIENT_RESOURCES {
		log.Error(nil, "error creating new compute instance, reusing", "ci", ci, "error", ret)
	}

	migInfos, err := populateMigDeviceInfos(device)
	if err != nil {
		return nil, fmt.Errorf("failed to populate MIG device infos: %v", err)
	}

	return migInfos, nil
}

// populateMigDeviceInfos walks through MIG devices and populates device info
func populateMigDeviceInfos(device nvml.Device) (map[string]*MigDeviceInfo, error) {
	migInfos := make(map[string]*MigDeviceInfo)

	err := walkMigDevices(device, func(i int, migDevice nvml.Device) error {
		parentUuid, ret := device.GetUUID()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting parent GPU UUID: %v", ret)
		}

		giID, ret := migDevice.GetGpuInstanceId()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting GPU instance ID for MIG device: %v", ret)
		}

		gi, ret := device.GetGpuInstanceById(giID)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting GPU instance for '%v': %v", giID, ret)
		}

		giInfo, ret := gi.GetInfo()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting GPU instance info for '%v': %v", giID, ret)
		}

		ciID, ret := migDevice.GetComputeInstanceId()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting Compute instance ID for MIG device: %v", ret)
		}

		ci, ret := gi.GetComputeInstanceById(ciID)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting Compute instance for '%v': %v", ciID, ret)
		}

		ciInfo, ret := ci.GetInfo()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting Compute instance info for '%v': %v", ciID, ret)
		}

		uuid, ret := migDevice.GetUUID()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting UUID for MIG device: %v", ret)
		}

		migInfos[uuid] = &MigDeviceInfo{
			uuid:   parentUuid,
			giInfo: &giInfo,
			ciInfo: &ciInfo,
			start:  int32(giInfo.Placement.Start),
			size:   int32(giInfo.Placement.Size),
		}

		return nil
	})

	return migInfos, err
}

// walkMigDevices iterates through all MIG devices on a GPU
func walkMigDevices(d nvml.Device, f func(i int, d nvml.Device) error) error {
	count, ret := d.GetMaxMigDeviceCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("error getting max MIG device count: %v", ret)
	}

	for i := 0; i < count; i++ {
		device, ret := d.GetMigDeviceHandleByIndex(i)
		if ret == nvml.ERROR_NOT_FOUND {
			continue
		}
		if ret == nvml.ERROR_INVALID_ARGUMENT {
			continue
		}
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting MIG device handle at index '%v': %v", i, ret)
		}
		err := f(i, device)
		if err != nil {
			return err
		}
	}
	return nil
}

// discoverAvailableProfilesOnGpus discovers GPU profiles and capabilities
func (c *DaemonsetController) discoverAvailableProfilesOnGpus(instaslice *inferencev1alpha1.Instaslice) (*inferencev1alpha1.Instaslice, nvml.Return, bool, error) {
	log := klog.FromContext(context.TODO())
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, ret, false, fmt.Errorf("error getting device count: %v", ret)
	}

	nodeGPUs := make([]inferencev1alpha1.DiscoveredGPU, count)
	discoverProfilePerNode := true
	var memory nvml.Memory

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, ret, false, fmt.Errorf("error getting device handle: %v", ret)
		}

		uuid, _ := device.GetUUID()
		mode, _, ret := device.GetMigMode()
		if ret == nvml.ERROR_NOT_SUPPORTED {
			return instaslice, ret, false, fmt.Errorf("unable to detect mig mode")
		}
		if ret != nvml.SUCCESS {
			return instaslice, ret, false, fmt.Errorf("error getting MIG mode: %v", ret)
		}
		if mode != nvml.DEVICE_MIG_ENABLE {
			log.V(2).Info("mig mode not enabled on gpu", "uuid", uuid)
			continue
		}

		memory, ret = device.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			return nil, ret, false, fmt.Errorf("error getting memory info: %v", ret)
		}

		gpuName, _ := device.GetName()
		nodeGPUs[i].GPUUUID = uuid
		nodeGPUs[i].GPUName = gpuName
		nodeGPUs[i].GPUMemory = *resource.NewQuantity(int64(memory.Total), resource.BinarySI)
		discoveredGpusOnHost = append(discoveredGpusOnHost, uuid)

		if discoverProfilePerNode {
			if err := c.discoverMigProfiles(device, instaslice, memory.Total); err != nil {
				return nil, 0, true, err
			}
			discoverProfilePerNode = false
		}
		instaslice.Status.NodeResources.NodeGPUs = nodeGPUs
	}
	return instaslice, ret, false, nil
}

// discoverMigProfiles discovers available MIG profiles on a GPU
func (c *DaemonsetController) discoverMigProfiles(device nvml.Device, instaslice *inferencev1alpha1.Instaslice, totalMemory uint64) error {
	for j := 0; j < nvml.GPU_INSTANCE_PROFILE_COUNT; j++ {
		giProfileInfo, ret := device.GetGpuInstanceProfileInfo(j)
		if ret == nvml.ERROR_NOT_SUPPORTED || ret == nvml.ERROR_INVALID_ARGUMENT {
			continue
		}
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting GPU instance profile info: %v", ret)
		}

		profile := NewMigProfile(j, j, nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_SHARED,
			giProfileInfo.SliceCount, giProfileInfo.SliceCount, giProfileInfo.MemorySizeMB, totalMemory)

		giPossiblePlacements, ret := device.GetGpuInstancePossiblePlacements(&giProfileInfo)
		if ret == nvml.ERROR_NOT_SUPPORTED || ret == nvml.ERROR_INVALID_ARGUMENT {
			continue
		}
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting possible placements: %v", ret)
		}

		placementsForProfile := []inferencev1alpha1.Placement{}
		for _, p := range giPossiblePlacements {
			placement := inferencev1alpha1.Placement{
				Size:  int32(p.Size),
				Start: int32(p.Start),
			}
			placementsForProfile = append(placementsForProfile, placement)
		}

		aggregatedPlacementsForProfile := inferencev1alpha1.Mig{
			Placements:     placementsForProfile,
			GIProfileID:    int32(j),
			CIProfileID:    int32(profile.CIProfileID),
			CIEngProfileID: int32(profile.CIEngProfileID),
		}

		if instaslice.Status.NodeResources.MigPlacement == nil {
			instaslice.Status.NodeResources.MigPlacement = make(map[string]inferencev1alpha1.Mig)
		}
		instaslice.Status.NodeResources.MigPlacement[profile.String()] = aggregatedPlacementsForProfile
	}
	return nil
}

// NewMigProfile constructs a new MigProfile struct
func NewMigProfile(giProfileID, ciProfileID, ciEngProfileID int, giSliceCount, ciSliceCount uint32, migMemorySizeMB, totalDeviceMemoryBytes uint64) *MigProfile {
	return &MigProfile{
		C:              int(ciSliceCount),
		G:              int(giSliceCount),
		GB:             int(getMigMemorySizeInGB(totalDeviceMemoryBytes, migMemorySizeMB)),
		GIProfileID:    giProfileID,
		CIProfileID:    ciProfileID,
		CIEngProfileID: ciEngProfileID,
	}
}

// getMigMemorySizeInGB calculates GPU memory size in GBs
func getMigMemorySizeInGB(totalDeviceMemory, migMemorySizeMB uint64) uint64 {
	const fracDenominator = 8
	const oneMB = 1024 * 1024
	const oneGB = 1024 * 1024 * 1024
	fractionalGpuMem := (float64(migMemorySizeMB) * oneMB) / float64(totalDeviceMemory)
	fractionalGpuMem = math.Ceil(fractionalGpuMem*fracDenominator) / fracDenominator
	totalMemGB := float64((totalDeviceMemory + oneGB - 1) / oneGB)
	return uint64(math.Round(fractionalGpuMem * totalMemGB))
}

// String returns the string representation of a MigProfile
func (m MigProfile) String() string {
	var suffix string
	if len(m.Attributes()) > 0 {
		suffix = "+" + strings.Join(m.Attributes(), ",")
	}
	if m.C == m.G {
		return fmt.Sprintf("%dg.%dgb%s", m.G, m.GB, suffix)
	}
	return fmt.Sprintf("%dc.%dg.%dgb%s", m.C, m.G, m.GB, suffix)
}

// Attributes returns the list of attributes associated with a MigProfile
func (m MigProfile) Attributes() []string {
	var attr []string
	switch m.GIProfileID {
	case nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1:
		attr = append(attr, constants.AttributeMediaExtensions)
	}
	return attr
}

// CalculateTotalMemoryGB calculates the total GPU memory in GB from discovered GPUs
func CalculateTotalMemoryGB(nodeGPUs []inferencev1alpha1.DiscoveredGPU) (float64, error) {
	var totalMemoryGB float64
	for _, gpu := range nodeGPUs {
		memoryBytes := gpu.GPUMemory.Value()
		memoryGB := float64(memoryBytes) / (1024 * 1024 * 1024)
		totalMemoryGB += memoryGB
	}
	return totalMemoryGB, nil
}
