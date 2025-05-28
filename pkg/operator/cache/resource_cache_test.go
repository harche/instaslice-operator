package cache

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	corelisters "k8s.io/client-go/listers/core/v1"
)

// fakePodLister implements PodLister for tests
type fakePodLister struct {
	pods []*v1.Pod
}

func (f *fakePodLister) List(_ labels.Selector) ([]*v1.Pod, error) {
	return f.pods, nil
}
func (f *fakePodLister) Pods(namespace string) corelisters.PodNamespaceLister {
	panic("not implemented")
}

// fakeNodeLister implements NodeLister for tests
type fakeNodeLister struct {
	nodes []*v1.Node
}

func (f *fakeNodeLister) List(_ labels.Selector) ([]*v1.Node, error) {
	return f.nodes, nil
}
func (f *fakeNodeLister) Get(name string) (*v1.Node, error) {
	for _, n := range f.nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return nil, fmt.Errorf("node %q not found", name)
}

func newNode(name, cpu, mem, storage, ephemeral string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{Allocatable: v1.ResourceList{
			v1.ResourceCPU:              resource.MustParse(cpu),
			v1.ResourceMemory:           resource.MustParse(mem),
			v1.ResourceStorage:          resource.MustParse(storage),
			v1.ResourceEphemeralStorage: resource.MustParse(ephemeral),
		}},
	}
}

func newPod(ns, name, node, cpu, mem, storage, ephemeral string, phase v1.PodPhase) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(ns + "-" + name)},
		Spec: v1.PodSpec{NodeName: node, Containers: []v1.Container{{
			Name: "c", Image: "pause", Resources: v1.ResourceRequirements{Requests: v1.ResourceList{
				v1.ResourceCPU:              resource.MustParse(cpu),
				v1.ResourceMemory:           resource.MustParse(mem),
				v1.ResourceStorage:          resource.MustParse(storage),
				v1.ResourceEphemeralStorage: resource.MustParse(ephemeral),
			}},
		}}},
		Status: v1.PodStatus{Phase: phase},
	}
}

func n(name string) *v1.Node { return newNode(name, "4000m", "8Gi", "100Gi", "50Gi") }
func qty(s string) int64     { q := resource.MustParse(s); return (&q).Value() }

func TestNodeHandlers(t *testing.T) {
	rc := NewResourceCache()
	h := rc.ResourceEventHandlerForNode()
	n1 := newNode("worker1", "4000m", "8Gi", "100Gi", "50Gi")
	h.AddFunc(n1)
	if got := len(rc.nodes); got != 1 {
		t.Fatalf("expected 1 node after add, got %d", got)
	}
	n2 := n1.DeepCopy()
	n2.Status.Allocatable[v1.ResourceCPU] = resource.MustParse("2000m")
	n2.Status.Allocatable[v1.ResourceMemory] = resource.MustParse("6Gi")
	n2.Status.Allocatable[v1.ResourceStorage] = resource.MustParse("80Gi")
	n2.Status.Allocatable[v1.ResourceEphemeralStorage] = resource.MustParse("30Gi")
	h.UpdateFunc(n1, n2)
	rc.RLock()
	alloc := rc.nodes["worker1"].Allocatable
	rc.RUnlock()
	if alloc.MilliCPU != 2000 {
		t.Errorf("CPU allocatable not updated: got %d, want %d", alloc.MilliCPU, 2000)
	}
	if alloc.Memory != qty("6Gi") {
		t.Errorf("Memory allocatable not updated: got %d, want %d", alloc.Memory, qty("6Gi"))
	}
	if alloc.Storage != qty("80Gi") {
		t.Errorf("Storage allocatable not updated: got %d, want %d", alloc.Storage, qty("80Gi"))
	}
	if alloc.EphemeralStorage != qty("30Gi") {
		t.Errorf("Ephemeral storage allocatable not updated: got %d, want %d", alloc.EphemeralStorage, qty("30Gi"))
	}
	h.DeleteFunc(n2)
	if got := len(rc.nodes); got != 0 {
		t.Fatalf("expected 0 nodes after delete, got %d", got)
	}
}

func TestPodLifecycleHandlers(t *testing.T) {
	rc := NewResourceCache()
	rc.ResourceEventHandlerForNode().AddFunc(n("nodeA"))
	ph := rc.ResourceEventHandlerForPod()
	type step struct {
		name           string
		oldPod, newPod *v1.Pod
		expect         LocalResource
		expectFitCheck *v1.Pod
		expectFit      bool
	}
	p1Pending := newPod("ns", "p1", "", "1000m", "1Gi", "10Gi", "5Gi", v1.PodPending)
	p1Bound := p1Pending.DeepCopy()
	p1Bound.Spec.NodeName = "nodeA"
	p1Bound.Status.Phase = v1.PodRunning
	p1Resized := p1Bound.DeepCopy()
	p1Resized.Spec.Containers[0].Resources.Requests[v1.ResourceEphemeralStorage] = resource.MustParse("8Gi")
	p1Succeeded := p1Resized.DeepCopy()
	p1Succeeded.Status.Phase = v1.PodSucceeded
	p1Failed := p1Resized.DeepCopy()
	p1Failed.Status.Phase = v1.PodFailed
	p2 := newPod("ns", "p2", "nodeA", "3000m", "6Gi", "80Gi", "40Gi", v1.PodRunning)
	steps := []step{
		{name: "add pending pod", newPod: p1Pending, expect: LocalResource{}, expectFitCheck: newPod("ns", "probe", "nodeA", "4000m", "7Gi", "90Gi", "40Gi", v1.PodRunning), expectFit: true},
		{name: "bind pod to node", oldPod: p1Pending, newPod: p1Bound, expect: LocalResource{MilliCPU: 1000, Memory: qty("1Gi"), Storage: qty("10Gi"), EphemeralStorage: qty("5Gi")}},
		{name: "add second pod", newPod: p2, expect: LocalResource{MilliCPU: 4000, Memory: qty("7Gi"), Storage: qty("90Gi"), EphemeralStorage: qty("45Gi")}, expectFitCheck: newPod("ns", "too-big", "nodeA", "1000m", "1Mi", "20Gi", "10Gi", v1.PodRunning), expectFit: false},
		{name: "resize first pod ephemeral storage", oldPod: p1Bound, newPod: p1Resized, expect: LocalResource{MilliCPU: 4000, Memory: qty("7Gi"), Storage: qty("90Gi"), EphemeralStorage: qty("48Gi")}},
		{name: "mark first pod as failed", oldPod: p1Resized, newPod: p1Failed, expect: LocalResource{MilliCPU: 3000, Memory: qty("6Gi"), Storage: qty("80Gi"), EphemeralStorage: qty("40Gi")}},
		{name: "delete failed pod", oldPod: p1Failed, newPod: nil, expect: LocalResource{MilliCPU: 3000, Memory: qty("6Gi"), Storage: qty("80Gi"), EphemeralStorage: qty("40Gi")}},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			if s.oldPod == nil && s.newPod != nil {
				ph.AddFunc(s.newPod)
			} else if s.oldPod != nil && s.newPod != nil {
				ph.UpdateFunc(s.oldPod, s.newPod)
			} else if s.oldPod != nil && s.newPod == nil {
				ph.DeleteFunc(s.oldPod)
			}
			rc.RLock()
			got := rc.nodes["nodeA"].Requested
			rc.RUnlock()
			if got != s.expect {
				t.Errorf("%s: expected %+v, got %+v", s.name, s.expect, got)
			}
			if s.expectFitCheck != nil {
				if fit := rc.Fits(s.expectFitCheck.Spec.NodeName, s.expectFitCheck); fit != s.expectFit {
					t.Errorf("%s: expected fit=%v, got %v", s.name, s.expectFit, fit)
				}
			}
		})
	}
}
