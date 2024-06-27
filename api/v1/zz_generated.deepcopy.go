//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AllocationDetails) DeepCopyInto(out *AllocationDetails) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AllocationDetails.
func (in *AllocationDetails) DeepCopy() *AllocationDetails {
	if in == nil {
		return nil
	}
	out := new(AllocationDetails)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Instaslice) DeepCopyInto(out *Instaslice) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Instaslice.
func (in *Instaslice) DeepCopy() *Instaslice {
	if in == nil {
		return nil
	}
	out := new(Instaslice)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Instaslice) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstasliceList) DeepCopyInto(out *InstasliceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Instaslice, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstasliceList.
func (in *InstasliceList) DeepCopy() *InstasliceList {
	if in == nil {
		return nil
	}
	out := new(InstasliceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *InstasliceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstasliceSpec) DeepCopyInto(out *InstasliceSpec) {
	*out = *in
	if in.MigGPUUUID != nil {
		in, out := &in.MigGPUUUID, &out.MigGPUUUID
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Allocations != nil {
		in, out := &in.Allocations, &out.Allocations
		*out = make(map[string]AllocationDetails, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Prepared != nil {
		in, out := &in.Prepared, &out.Prepared
		*out = make(map[string]PreparedDetails, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Migplacement != nil {
		in, out := &in.Migplacement, &out.Migplacement
		*out = make([]Mig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstasliceSpec.
func (in *InstasliceSpec) DeepCopy() *InstasliceSpec {
	if in == nil {
		return nil
	}
	out := new(InstasliceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstasliceStatus) DeepCopyInto(out *InstasliceStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstasliceStatus.
func (in *InstasliceStatus) DeepCopy() *InstasliceStatus {
	if in == nil {
		return nil
	}
	out := new(InstasliceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Mig) DeepCopyInto(out *Mig) {
	*out = *in
	if in.Placements != nil {
		in, out := &in.Placements, &out.Placements
		*out = make([]Placement, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Mig.
func (in *Mig) DeepCopy() *Mig {
	if in == nil {
		return nil
	}
	out := new(Mig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Placement) DeepCopyInto(out *Placement) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Placement.
func (in *Placement) DeepCopy() *Placement {
	if in == nil {
		return nil
	}
	out := new(Placement)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PreparedDetails) DeepCopyInto(out *PreparedDetails) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PreparedDetails.
func (in *PreparedDetails) DeepCopy() *PreparedDetails {
	if in == nil {
		return nil
	}
	out := new(PreparedDetails)
	in.DeepCopyInto(out)
	return out
}
