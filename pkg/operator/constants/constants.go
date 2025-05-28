package constants

import "time"

// Scheduling and gating constants
const (
	GateName                  = "instaslice.redhat.com/gpu-allocation"
	FinalizerName             = "instaslice.redhat.com/finalizer"
	NodeLabel                 = "kubernetes.io/hostname"
	ManagedLabel              = "instaslice.redhat.com/managed"
	InstasliceManagedTrue     = "true"
	MigCapableTrue            = "true"
)

// Resource and namespace constants
const (
	InstaSliceOperatorNamespace = "instaslice-operator"
	InstasliceDaemonsetName     = "controller-daemonset"
	ServiceAccountName          = "instaslice-operator"
	DaemonSetName               = "daemonset"
)

// Resource prefixes and names
const (
	NvidiaMIGPrefix     = "nvidia.com/mig-"
	OrgInstaslicePrefix = "instaslice.redhat.com/"
	QuotaResourceName   = "instaslice.redhat.com/accelerator-memory-quota"
	GPUMemoryLabelName  = "instaslice.redhat.com/gpu-memory"
	GPUCountLabelName   = "instaslice.redhat.com/gpu-count"
)

// NVML and GPU constants
const (
	AttributeMediaExtensions = "me"
)

// Timing constants
const (
	Requeue1sDelay = 1 * time.Second
	Requeue2sDelay = 2 * time.Second
)

// Error messages
const (
	NoContainerInsidePodErr          = "no container found inside pod"
	MultipleContainersUnsupportedErr = "multiple containers in pod not supported"
)