package cache

import (
   "context"
   "fmt"
   "time"

   "k8s.io/apimachinery/pkg/util/wait"
   "k8s.io/client-go/informers"
   "k8s.io/client-go/kubernetes"
   "k8s.io/client-go/rest"
   utilcache "k8s.io/client-go/tools/cache"
   "k8s.io/klog/v2"
   ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
)

// ResourceTracker periodically rebuilds the resource cache and subscribes to events
var _ ctrlmgr.Runnable = (*ResourceTracker)(nil)

// ResourceTracker manages resource cache using informers
type ResourceTracker struct {
   cache   *ResourceCache
   factory informers.SharedInformerFactory
}

// NewResourceTracker creates a new ResourceTracker
func NewResourceTracker(cfg *rest.Config) (*ResourceTracker, error) {
   cs, err := kubernetes.NewForConfig(cfg)
   if err != nil {
       return nil, err
   }
   f := informers.NewSharedInformerFactory(cs, 0)
   t := &ResourceTracker{cache: NewResourceCache(), factory: f}
   if _, err := f.Core().V1().Nodes().Informer().AddEventHandler(t.cache.ResourceEventHandlerForNode()); err != nil {
       return nil, fmt.Errorf("failed to add node handler: %w", err)
   }
   if _, err := f.Core().V1().Pods().Informer().AddEventHandler(t.cache.ResourceEventHandlerForPod()); err != nil {
       return nil, fmt.Errorf("failed to add pod handler: %w", err)
   }
   return t, nil
}

// Start runs the informer loop and cache rebuild
func (t *ResourceTracker) Start(ctx context.Context) error {
   t.factory.Start(ctx.Done())
   if ok := utilcache.WaitForCacheSync(ctx.Done(),
       t.factory.Core().V1().Pods().Informer().HasSynced,
       t.factory.Core().V1().Nodes().Informer().HasSynced); !ok {
       return fmt.Errorf("ResourceTracker: informers did not sync")
   }
   if err := t.cache.Rebuild(
       t.factory.Core().V1().Nodes().Lister(),
       t.factory.Core().V1().Pods().Lister()); err != nil {
       return err
   }
   go wait.Until(func() {
       if err := t.cache.Rebuild(
           t.factory.Core().V1().Nodes().Lister(),
           t.factory.Core().V1().Pods().Lister()); err != nil {
           klog.InfoS("failed to rebuild the cache during periodic reconcile", "err", err)
       }
       t.cache.DebugDump()
   }, 10*time.Minute, ctx.Done())
   <-ctx.Done()
   return nil
}

// Cache returns the internal ResourceCache
func (t *ResourceTracker) Cache() *ResourceCache {
   return t.cache
}