package deviceplugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	instaclient "github.com/openshift/instaslice-operator/pkg/generated/clientset/versioned"
	"k8s.io/client-go/rest"

	nvml "github.com/NVIDIA/go-nvml/pkg/nvml"
	nvcdi "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi"
	nvcdispec "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi/spec"

	"golang.org/x/sys/unix"
	"tags.cncf.io/container-device-interface/pkg/cdi"
	parser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	// UUID generator for unique spec filenames
	utiluuid "k8s.io/apimachinery/pkg/util/uuid"

	instav1 "github.com/openshift/instaslice-operator/pkg/apis/dasoperator/v1alpha1"
)

var _ pluginapi.DevicePluginServer = (*Server)(nil)

type Server struct {
	pluginapi.UnimplementedDevicePluginServer
	Manager          *Manager
	SocketPath       string
	InstasliceClient instaclient.Interface
	NodeName         string
	EmulatedMode     instav1.EmulatedMode
	allocMutex       sync.Mutex
}

const allocationAnnotationKey = "mig.das.com/allocation"

func NewServer(mgr *Manager, socketPath string, kubeConfig *rest.Config, emulatedMode instav1.EmulatedMode) (*Server, error) {
	cfg := rest.CopyConfig(kubeConfig)
	cfg.AcceptContentTypes = "application/json"
	cfg.ContentType = "application/json"
	client, err := instaclient.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create instaslice client: %w", err)
	}
	nodeName := os.Getenv("NODE_NAME")
	return &Server{Manager: mgr, SocketPath: socketPath, InstasliceClient: client, NodeName: nodeName, EmulatedMode: emulatedMode}, nil
}

func (s *Server) Start(ctx context.Context) error {
	klog.InfoS("Starting device plugin server", "socket", s.SocketPath)
	// CDI cache is configured during daemonset initialization

	// remove existing socket file, if any
	if err := os.Remove(s.SocketPath); err != nil {
		if os.IsNotExist(err) {
			klog.InfoS("Socket file does not exist, skipping removal", "socket", s.SocketPath)
		} else {
			klog.ErrorS(err, "Failed to remove existing socket file", "socket", s.SocketPath)
			return fmt.Errorf("failed to remove existing socket %q: %w", s.SocketPath, err)
		}
	} else {
		klog.InfoS("Removed existing socket file", "socket", s.SocketPath)
	}
	lis, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		klog.ErrorS(err, "Failed to listen on socket", "socket", s.SocketPath)
		return fmt.Errorf("failed to listen on socket %q: %w", s.SocketPath, err)
	}
	klog.InfoS("Listening on socket", "socket", s.SocketPath)
	grpcServer := grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(grpcServer, s)
	klog.InfoS("Registered device plugin server", "socket", s.SocketPath)
	klog.InfoS("Starting device manager", "resource", s.Manager.ResourceName)
	go s.Manager.Start(ctx)
	klog.InfoS("Starting gRPC server", "socket", s.SocketPath)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			klog.ErrorS(err, "gRPC server stopped unexpectedly", "socket", s.SocketPath)
		} else {
			klog.InfoS("gRPC server stopped", "socket", s.SocketPath)
		}
	}()
	go func() {
		<-ctx.Done()
		klog.InfoS("Shutting down device plugin server", "socket", s.SocketPath)
		grpcServer.Stop()
	}()
	return nil
}

// GetDevicePluginOptions returns the options supported by the device plugin.
func (s *Server) GetDevicePluginOptions(ctx context.Context, req *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// ListAndWatch streams the list of devices, sending initial list and subsequent updates.
func (s *Server) ListAndWatch(req *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	klog.InfoS("ListAndWatch started", "resource", s.Manager.ResourceName)

	// Refresh the CDI cache to make sure we see all specs on disk.
	if err := cdi.Refresh(); err != nil {
		klog.ErrorS(err, "failed to refresh CDI cache")
	}

	// build initial device list from existing CDI spec files
	var initDevices []*pluginapi.Device
	cache := cdi.GetDefaultCache()
	for _, vendor := range cache.ListVendors() {
		klog.V(4).InfoS("Enumerating CDI specs", "vendor", vendor)
		for _, spec := range cache.GetVendorSpecs(vendor) {
			id := filepath.Base(spec.GetPath())
			klog.V(4).InfoS("Adding device from spec", "path", spec.GetPath(), "id", id)
			initDevices = append(initDevices, &pluginapi.Device{ID: id, Health: pluginapi.Healthy})
		}
	}

	// send initial device list
	if err := stream.Send(&pluginapi.ListAndWatchResponse{Devices: initDevices}); err != nil {
		return fmt.Errorf("failed to send initial device list: %w", err)
	}

	// stream updates
	for {
		select {
		case <-stream.Context().Done():
			klog.InfoS("ListAndWatch stopped", "resource", s.Manager.ResourceName)
			return nil
		case devs := <-s.Manager.Updates():
			klog.InfoS("ListAndWatch sending update", "resource", s.Manager.ResourceName, "devices", devs)
			if err := stream.Send(&pluginapi.ListAndWatchResponse{Devices: devs}); err != nil {
				return fmt.Errorf("failed to send device update: %w", err)
			}
		}
	}
}

func (s *Server) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	klog.InfoS("Received Allocate request", "containerRequests", req.GetContainerRequests(), "emulatedMode", s.EmulatedMode)
	count := len(req.GetContainerRequests())
	resp := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, count),
	}

	allocations, err := s.getAllocationsByNodeGPU(ctx, s.NodeName, s.Manager.ResourceName, count)
	if err != nil {
		klog.ErrorS(err, "failed to get allocations", "node", s.NodeName, "profile", s.Manager.ResourceName)
	} else {
		klog.InfoS("Fetched allocations for Allocate", "count", len(allocations), "node", s.NodeName, "profile", s.Manager.ResourceName)
		klog.V(5).InfoS("Allocations", "allocations", allocations)
	}

	for i := 0; i < count; i++ {
		resp.ContainerResponses[i] = &pluginapi.ContainerAllocateResponse{}

		ids := req.GetContainerRequests()[i].GetDevicesIDs()
		if len(ids) == 0 {
			ids = []string{string(utiluuid.NewUUID())}
		}

		var annotations map[string]string
		var envVar string
		if i < len(allocations) {
			alloc := allocations[i]
			if s.EmulatedMode == instav1.EmulatedModeDisabled {
				uuid, err := s.createMigSlice(ctx, alloc)
				if err != nil {
					klog.ErrorS(err, "failed to create MIG slice")
					return nil, err
				}
				envVar = fmt.Sprintf("NVIDIA_VISIBLE_DEVICES=%s", uuid)
			} else {
				envVar = "NVIDIA_VISIBLE_DEVICES=test"
			}
			if data, err := json.Marshal(alloc); err == nil {
				annotations = map[string]string{allocationAnnotationKey: string(data)}
			} else {
				klog.ErrorS(err, "failed to marshal allocation")
			}
		} else {
			envVar = "NVIDIA_VISIBLE_DEVICES=test"
		}

		for _, id := range ids {
			_, cdiDevices, err := WriteCDISpecForResource(s.Manager.ResourceName, id, annotations, envVar, s.EmulatedMode)
			if err != nil {
				return nil, err
			}
			resp.ContainerResponses[i].CDIDevices = append(resp.ContainerResponses[i].CDIDevices, cdiDevices...)
		}

		// After CDI spec creation succeed we can mark the AllocationClaim
		// as in use so the scheduler does not hand it out again.

		if i < len(allocations) {
			alloc := allocations[i]
			alloc.Status.State = instav1.AllocationClaimStatusInUse
			updated := alloc
			if s.InstasliceClient != nil {
				var err error
				updated, err = UpdateAllocationStatus(ctx, s.InstasliceClient, alloc, instav1.AllocationClaimStatusInUse)
				if err != nil {
					klog.ErrorS(err, "failed to update allocation status", "allocation", alloc.Name, "status", instav1.AllocationClaimStatusInUse)
					return nil, err
				}
			} else {
				cond := metav1.Condition{
					Type:               "State",
					Status:             metav1.ConditionTrue,
					Reason:             string(instav1.AllocationClaimStatusInUse),
					Message:            fmt.Sprintf("Allocation is %s", instav1.AllocationClaimStatusInUse),
					ObservedGeneration: alloc.Generation,
				}
				meta.SetStatusCondition(&updated.Status.Conditions, cond)
			}
			s.allocMutex.Lock()
			if err := allocationIndexer.Update(updated); err != nil {
				klog.ErrorS(err, "failed to update allocation in indexer", "allocation", updated.Name)
			}
			s.allocMutex.Unlock()
		}
	}

	return resp, nil
}

func (s *Server) PreStartContainer(ctx context.Context, req *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

// createMigSlice uses the NVML library to create a MIG slice as specified by
// the AllocationClaim. It looks up the GI and CI profile IDs from the
// discovered node resources stored in the Manager.
func (s *Server) createMigSlice(ctx context.Context, alloc *instav1.AllocationClaim) (string, error) {
	spec, err := getAllocationClaimSpec(alloc)
	if err != nil {
		return "", err
	}
	mig, ok := s.Manager.resources.MigPlacement[spec.Profile]
	if !ok {
		return "", fmt.Errorf("profile %s not found", spec.Profile)
	}

	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return "", fmt.Errorf("nvml init failed: %v", ret)
	}
	defer nvml.Shutdown()

	dev, ret := nvml.DeviceGetHandleByUUID(spec.GPUUUID)
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("get device %s: %v", spec.GPUUUID, ret)
	}

	giInfo, ret := dev.GetGpuInstanceProfileInfo(int(mig.GIProfileID))
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("get GI profile info: %v", ret)
	}

	placement := nvml.GpuInstancePlacement{Start: uint32(spec.MigPlacement.Start), Size: uint32(spec.MigPlacement.Size)}
	gpuInst, ret := dev.CreateGpuInstanceWithPlacement(&giInfo, &placement)
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("create GPU instance: %v", ret)
	}

	ciInfo, ret := gpuInst.GetComputeInstanceProfileInfo(int(mig.CIProfileID), 0)
	if ret != nvml.SUCCESS {
		gpuInst.Destroy()
		return "", fmt.Errorf("get CI profile info: %v", ret)
	}

	ci, ret := gpuInst.CreateComputeInstance(&ciInfo)
	if ret != nvml.SUCCESS {
		gpuInst.Destroy()
		return "", fmt.Errorf("create compute instance: %v", ret)
	}

	uuid, err := migUUIDFromInstance(dev, gpuInst, ci)
	if err != nil {
		ci.Destroy()
		gpuInst.Destroy()
		return "", err
	}

	klog.InfoS("Created MIG slice", "gpu", spec.GPUUUID, "profile", spec.Profile, "migUUID", uuid)
	return uuid, nil
}

// migUUIDFromInstance finds the UUID of the MIG device corresponding to the
// given compute instance.
func migUUIDFromInstance(parent nvml.Device, gi nvml.GpuInstance, ci nvml.ComputeInstance) (string, error) {
	ciInfo, ret := ci.GetInfo()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("get compute instance info: %v", ret)
	}

	giInfo, ret := gi.GetInfo()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("get gpu instance info: %v", ret)
	}

	count, ret := parent.GetMaxMigDeviceCount()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("get MIG device count: %v", ret)
	}

	for i := 0; i < count; i++ {
		migDev, ret := parent.GetMigDeviceHandleByIndex(i)
		if ret != nvml.SUCCESS {
			continue
		}
		giID, ret := nvml.DeviceGetGpuInstanceId(migDev)
		if ret != nvml.SUCCESS {
			continue
		}
		ciID, ret := nvml.DeviceGetComputeInstanceId(migDev)
		if ret != nvml.SUCCESS {
			continue
		}
		if uint32(giID) == giInfo.Id && uint32(ciID) == ciInfo.Id {
			uuid, ret := migDev.GetUUID()
			if ret != nvml.SUCCESS {
				return "", fmt.Errorf("get MIG UUID: %v", ret)
			}
			return uuid, nil
		}
	}

	return "", fmt.Errorf("matching MIG device not found")
}

func profileFromResourceName(res string) string {
	parts := strings.Split(res, "/")
	return parts[len(parts)-1]
}

func (s *Server) getAllocationsByNodeGPU(ctx context.Context, nodeName, profileName string, count int) ([]*instav1.AllocationClaim, error) {
	if count <= 0 {
		return nil, fmt.Errorf("requested allocation count must be greater than zero")
	}
	if allocationIndexer == nil {
		return nil, fmt.Errorf("allocation indexer not initialized")
	}

	migProfile := profileFromResourceName(profileName)
	migProfile = unsanitizeProfileName(migProfile)
	key := fmt.Sprintf("%s/%s", nodeName, migProfile)
	var result []*instav1.AllocationClaim

	// First fetch the requested AllocationClaims from the indexer. We retry
	// a few times in case the informer cache hasn't synced yet.
	err := wait.ExponentialBackoff(wait.Backoff{Duration: 100 * time.Millisecond, Factor: 2, Steps: 5}, func() (bool, error) {
		s.allocMutex.Lock()
		defer s.allocMutex.Unlock()

		objs, err := allocationIndexer.ByIndex("node-MigProfile", key)
		if err != nil {
			return false, err
		}

		out := make([]*instav1.AllocationClaim, 0, len(objs))
		for _, obj := range objs {
			if a, ok := obj.(*instav1.AllocationClaim); ok {
				spec, err := getAllocationClaimSpec(a)
				if err != nil {
					klog.ErrorS(err, "failed to decode allocation spec")
					continue
				}
				if spec.Profile == migProfile && a.Status.State == instav1.AllocationClaimStatusCreated {
					out = append(out, a)
					if len(out) == count {
						break
					}
				}
			}
		}
		result = out
		return len(result) >= count, nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			return nil, fmt.Errorf("requested %d allocations but only found %d", count, len(result))
		}
		return nil, err
	}

	// Update each claim status to Processing outside of the fetch loop so
	// failures here don't masquerade as cache lookup errors.
	for _, a := range result[:count] {
		a.Status.State = instav1.AllocationClaimStatusProcessing
		if s.InstasliceClient != nil {
			if _, err := UpdateAllocationStatus(ctx, s.InstasliceClient, a, instav1.AllocationClaimStatusProcessing); err != nil {
				klog.ErrorS(err, "failed to update allocation status", "allocation", a.Name, "status", instav1.AllocationClaimStatusProcessing)
				return nil, fmt.Errorf("failed to update allocation status for %q: %w", a.Name, err)
			}
		}

		s.allocMutex.Lock()
		if err := allocationIndexer.Update(a); err != nil {
			klog.ErrorS(err, "failed to update allocation in indexer", "allocation", a.Name)
		}
		s.allocMutex.Unlock()

		klog.InfoS("Updated allocation status to Processing", "allocation", a.Name, "status", a.Status.State)
	}

	return result[:count], nil
}

// deviceNodesForMIG returns the device nodes corresponding to the MIG device with the given UUID.
// Errors are logged and returned so the caller can decide how to proceed.
func deviceNodeFromPath(path string) (*cdispec.DeviceNode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat device node: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("failed to get stat_t for %s", path)
	}
	mode := info.Mode()
	return &cdispec.DeviceNode{
		Path:        path,
		HostPath:    path,
		Type:        "c",
		Major:       int64(unix.Major(stat.Rdev)),
		Minor:       int64(unix.Minor(stat.Rdev)),
		FileMode:    &mode,
		Permissions: "rw",
	}, nil
}

// deviceNodesForMIG returns the device nodes corresponding to the MIG device with the given UUID.
// Errors are logged and returned so the caller can decide how to proceed.
func deviceNodesForMIG(uuid string) ([]*cdispec.DeviceNode, error) {
	if uuid == "" {
		return nil, fmt.Errorf("empty MIG UUID")
	}

	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("nvml init failed: %v", ret)
	}
	defer nvml.Shutdown()

	migDev, ret := nvml.DeviceGetHandleByUUID(uuid)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("get mig device by uuid: %v", ret)
	}

	minor, ret := nvml.DeviceGetMinorNumber(migDev)
	if ret != nvml.SUCCESS {
		parent, ret := nvml.DeviceGetDeviceHandleFromMigDeviceHandle(migDev)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("get parent device: %v", ret)
		}
		minor, ret = nvml.DeviceGetMinorNumber(parent)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("get minor number: %v", ret)
		}
	}

	paths := []string{
		fmt.Sprintf("/dev/nvidia%d", minor),
		"/dev/nvidiactl",
		// "/dev/nvidia-uvm" is intentionally omitted
		"/dev/nvidia-uvm-tools",
		"/dev/nvidia-modeset",
	}

	// include capability nodes if present
	if entries, err := os.ReadDir("/dev/nvidia-caps"); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "nvidia-cap") {
				paths = append(paths, filepath.Join("/dev/nvidia-caps", e.Name()))
			}
		}
	}

	var nodes []*cdispec.DeviceNode
	for _, p := range paths {
		if node, err := deviceNodeFromPath(p); err == nil {
			nodes = append(nodes, node)
		} else if !os.IsNotExist(err) {
			klog.ErrorS(err, "failed to get device node", "path", p)
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no device nodes found for MIG %s", uuid)
	}

	return nodes, nil
}

func prefixHostPaths(spec *cdispec.Spec, driverRoot string) {
	if spec == nil {
		return
	}
	for _, dn := range spec.ContainerEdits.DeviceNodes {
		if dn != nil && dn.HostPath != "" {
			dn.HostPath = filepath.Join(driverRoot, strings.TrimPrefix(dn.HostPath, "/"))
		}
	}
	for _, m := range spec.ContainerEdits.Mounts {
		if m != nil && m.HostPath != "" {
			m.HostPath = filepath.Join(driverRoot, strings.TrimPrefix(m.HostPath, "/"))
		}
	}
	for i := range spec.Devices {
		for _, dn := range spec.Devices[i].ContainerEdits.DeviceNodes {
			if dn != nil && dn.HostPath != "" {
				dn.HostPath = filepath.Join(driverRoot, strings.TrimPrefix(dn.HostPath, "/"))
			}
		}
		for _, m := range spec.Devices[i].ContainerEdits.Mounts {
			if m != nil && m.HostPath != "" {
				m.HostPath = filepath.Join(driverRoot, strings.TrimPrefix(m.HostPath, "/"))
			}
		}
	}
}

func removeDeviceNodePath(spec *cdispec.Spec, path string) {
	if spec == nil {
		return
	}
	var filtered []*cdispec.DeviceNode
	for _, dn := range spec.ContainerEdits.DeviceNodes {
		if dn == nil || dn.Path == path || dn.HostPath == path {
			continue
		}
		filtered = append(filtered, dn)
	}
	spec.ContainerEdits.DeviceNodes = filtered
	for i := range spec.Devices {
		filtered = nil
		for _, dn := range spec.Devices[i].ContainerEdits.DeviceNodes {
			if dn == nil || dn.Path == path || dn.HostPath == path {
				continue
			}
			filtered = append(filtered, dn)
		}
		spec.Devices[i].ContainerEdits.DeviceNodes = filtered
	}
}

// buildSpecFromNVCdi generates a CDI spec for the given MIG UUID using the
// NVIDIA nvcdi library. The generated spec is returned without being written to
// disk. Any annotations provided are applied to the device entry and a
// poststop hook is added to remove the spec at specPath. The returned spec is
// nil if generation fails.
func buildSpecFromNVCdi(kind, class, id string, annotations map[string]string, uuid, specPath string) (*cdispec.Spec, error) {
	lib, err := nvcdi.New(
		nvcdi.WithVendor(strings.Split(kind, "/")[0]),
		nvcdi.WithClass(class),
	)
	if err != nil {
		return nil, err
	}

	devSpecs, err := lib.GetDeviceSpecsByID(uuid)
	if err != nil {
		return nil, err
	}

	edits, err := lib.GetCommonEdits()
	if err != nil {
		return nil, err
	}

	specIF, err := nvcdispec.New(
		nvcdispec.WithDeviceSpecs(devSpecs),
		nvcdispec.WithEdits(*edits.ContainerEdits),
		nvcdispec.WithVendor(strings.Split(kind, "/")[0]),
		nvcdispec.WithClass(class),
	)
	if err != nil {
		return nil, err
	}

	spec := specIF.Raw()
	prefixHostPaths(spec, "/run/nvidia/driver")
	removeDeviceNodePath(spec, "/dev/nvidia-uvm")
	for i := range spec.Devices {
		spec.Devices[i].Name = id
		if spec.Devices[i].Annotations == nil {
			spec.Devices[i].Annotations = map[string]string{}
		}
		for k, v := range annotations {
			spec.Devices[i].Annotations[k] = v
		}
		spec.Devices[i].ContainerEdits.Env = append(spec.Devices[i].ContainerEdits.Env, fmt.Sprintf("MIG_UUID=%s", uuid))
		spec.Devices[i].ContainerEdits.Hooks = append(spec.Devices[i].ContainerEdits.Hooks, &cdispec.Hook{
			HookName: "poststop",
			Path:     "/bin/rm",
			Args:     []string{"-f", specPath},
		})
	}

	return spec, nil
}

// BuildCDIDevices builds a CDI spec and returns the spec object, spec name,
// spec path, and the corresponding CDIDevice slice. This helper is exported so
// that other packages (and tests) can generate CDI specs in a consistent way.
func BuildCDIDevices(kind, sanitizedClass, id string, annotations map[string]string, envVar string, emulated instav1.EmulatedMode) (*cdispec.Spec, string, string, []*pluginapi.CDIDevice) {
	specNameBase := fmt.Sprintf("%s_%s", sanitizedClass, id)
	specName := specNameBase + ".cdi.json"

	dynamicDir := cdi.DefaultStaticDir
	dirs := cdi.GetDefaultCache().GetSpecDirectories()
	if len(dirs) > 0 {
		dynamicDir = dirs[len(dirs)-1]
	}
	specPath := filepath.Join(dynamicDir, specName)

	// TODO - Do we need to create a CDI spec for each device Allocate request? can we not use a single spec for all devices of the same kind?
	var env []string
	var deviceNodes []*cdispec.DeviceNode
	if emulated == instav1.EmulatedModeEnabled {
		env = []string{"NVIDIA_VISIBLE_DEVICES=test", "CUDA_VISIBLE_DEVICES=test"}
		if envVar != "" {
			env = []string{envVar}
			if eq := strings.Index(envVar, "="); eq != -1 {
				val := envVar[eq+1:]
				env = append(env, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", val))
			}
		}
	} else {
		// envVar contains the MIG UUID in non-emulated mode
		if envVar != "" {
			uuid := strings.TrimPrefix(envVar, "NVIDIA_VISIBLE_DEVICES=")

			if spec, err := buildSpecFromNVCdi(kind, sanitizedClass, id, annotations, uuid, specPath); err == nil {
				cdiDevices := make([]*pluginapi.CDIDevice, len(spec.Devices))
				for j, dev := range spec.Devices {
					cdiDevices[j] = &pluginapi.CDIDevice{Name: fmt.Sprintf("%s=%s", kind, dev.Name)}
				}
				return spec, specName, specPath, cdiDevices
			} else {
				klog.ErrorS(err, "failed to generate CDI spec using nvcdi, falling back", "uuid", uuid)
			}

			env = []string{fmt.Sprintf("MIG_UUID=%s", uuid)}
			if nodes, err := deviceNodesForMIG(uuid); err == nil {
				deviceNodes = nodes
			} else {
				klog.ErrorS(err, "failed to get device nodes for MIG device", "uuid", uuid)
			}
		}
	}
	specObj := &cdispec.Spec{
		Version: cdispec.CurrentVersion,
		Kind:    kind,
		Devices: []cdispec.Device{
			{
				Name:        id,
				Annotations: annotations,
				ContainerEdits: cdispec.ContainerEdits{
					Env:         env,
					DeviceNodes: deviceNodes,
					Hooks: []*cdispec.Hook{
						{
							HookName: "poststop",
							Path:     "/bin/rm",
							Args:     []string{"-f", specPath},
						},
					},
				},
			},
		},
	}

	prefixHostPaths(specObj, "/run/nvidia/driver")
	removeDeviceNodePath(specObj, "/dev/nvidia-uvm")
	removeDeviceNodePath(specObj, "/dev/nvidia-uvm-tools")

	cdiDevices := make([]*pluginapi.CDIDevice, len(specObj.Devices))
	for j, dev := range specObj.Devices {
		cdiDevices[j] = &pluginapi.CDIDevice{
			Name: fmt.Sprintf("%s=%s", kind, dev.Name),
		}
	}
	return specObj, specName, specPath, cdiDevices
}

// WriteCDISpecForResource parses the given resource name, generates a CDI spec
// using BuildCDIDevices and writes it to the CDI cache. It returns the path to
// the written spec along with the generated CDIDevices.
func WriteCDISpecForResource(resourceName string, id string, annotations map[string]string, envVar string, emulated instav1.EmulatedMode) (string, []*pluginapi.CDIDevice, error) {
	vendor, class := parser.ParseQualifier(resourceName)
	sanitizedClass := class
	if err := parser.ValidateClassName(sanitizedClass); err != nil {
		sanitizedClass = "c" + sanitizedClass
	}
	kind := sanitizedClass
	if vendor != "" {
		kind = vendor + "/" + sanitizedClass
	}

	specObj, specName, specPath, cdiDevices := BuildCDIDevices(kind, sanitizedClass, id, annotations, envVar, emulated)

	// Wait for any previous spec with the same name to be removed. This is
	// important for transient specs tied to container lifecycles. The
	// removal is triggered by the poststop hook in the spec itself.
	// Wait up to three minutes, checking every second, for the old spec
	// file to disappear before writing the replacement.
	ctx := context.Background()
	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 3*time.Minute, false, func(ctx context.Context) (bool, error) {
		_, statErr := os.Stat(specPath)
		if statErr == nil {
			return false, nil
		}
		if errors.Is(statErr, os.ErrNotExist) {
			return true, nil
		}
		return false, statErr
	})
	if err != nil {
		return "", nil, fmt.Errorf("timed out waiting for old CDI spec %q to be removed: %w", specName, err)
	}

	if err := cdi.GetDefaultCache().WriteSpec(specObj, specName); err != nil {
		klog.ErrorS(err, "failed to write CDI spec", "name", specName)
		return "", nil, fmt.Errorf("failed to write CDI spec %q: %w", specName, err)
	}
	klog.InfoS("wrote CDI spec", "name", specName)

	return specPath, cdiDevices, nil
}

// writeCDISpecForResource is kept for backwards compatibility with older code.
// It simply calls the exported WriteCDISpecForResource function and discards the
// returned spec path.
func (s *Server) writeCDISpecForResource(resourceName string, id string) ([]*pluginapi.CDIDevice, error) {
	_, devices, err := WriteCDISpecForResource(resourceName, id, nil, "", s.EmulatedMode)
	return devices, err
}
