package informer

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

var deploymentGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

func newTestDeployment(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
	}}
}

func newTestDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{deploymentGVR: "DeploymentList"},
		objects...,
	)
}

func newTestManager(dc dynamic.Interface) *Manager {
	return &Manager{
		dynamicClient:       dc,
		logger:              zap.NewNop(),
		resyncPeriod:        time.Minute,
		healthCheckDuration: time.Second,
		healthCheckStopChan: make(chan struct{}),
		syncTimeout:         2 * time.Second,
		syncGracePeriod:     2 * time.Minute,
		baseRestartBackoff:  defaultBaseRestartBackoff,
		maxRestartBackoff:   defaultMaxRestartBackoff,
	}
}

func newTestRequestWatch(namespace, name string) *RequestWatch {
	return &RequestWatch{
		Req:                  ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: "test-es"}},
		ResourceName:         name,
		ResourceNamespace:    namespace,
		GroupVersionResource: &deploymentGVR,
		Handlers:             cache.ResourceEventHandlerFuncs{},
	}
}

func loadInfo(t *testing.T, m *Manager, key string) (info, bool) {
	t.Helper()
	value, ok := m.informers.Load(key)
	if !ok {
		return info{}, false
	}
	informerInfo, castOk := value.(info)
	if !castOk {
		t.Fatalf("informer map entry for %s has unexpected type %T", key, value)
	}
	return informerInfo, true
}

func TestAddStartsAndSyncsInformer(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	m := newTestManager(dc)
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)
	defer func() { _ = m.StopInformer(key) }()

	if err := m.Add(req); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	informerInfo, ok := loadInfo(t, m, key)
	if !ok {
		t.Fatal("informer not stored in map after Add")
	}
	if !informerInfo.Informer.HasSynced() {
		t.Fatal("informer not synced after Add returned")
	}
	if informerInfo.RestartCount != 0 {
		t.Fatalf("expected RestartCount 0, got %d", informerInfo.RestartCount)
	}

	// A second Add for the same key must be a no-op and must not replace the entry.
	if err := m.Add(req); err != nil {
		t.Fatalf("second Add failed: %v", err)
	}
	secondInfo, _ := loadInfo(t, m, key)
	if secondInfo.StopCh != informerInfo.StopCh {
		t.Fatal("second Add replaced the running informer")
	}
}

func TestAddFailsWhenTargetMissing(t *testing.T) {
	m := newTestManager(newTestDynamicClient())
	req := newTestRequestWatch("demo", "missing")

	if err := m.Add(req); err == nil {
		t.Fatal("expected Add to fail for missing target")
	}
	if _, ok := loadInfo(t, m, m.getKeyFromRequestWatch(req)); ok {
		t.Fatal("informer stored in map despite missing target")
	}
}

func TestAddCleansUpOnSyncTimeout(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	// Lists fail, so the informer can never complete its initial sync.
	dc.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list failure")
	})
	m := newTestManager(dc)
	m.syncTimeout = 300 * time.Millisecond
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)

	if err := m.Add(req); err == nil {
		t.Fatal("expected Add to fail on sync timeout")
	}
	if _, ok := loadInfo(t, m, key); ok {
		t.Fatal("unsynced informer left in map after sync timeout")
	}
}

func TestMonitorSkipsInformerWithinGracePeriod(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	dc.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list failure")
	})
	m := newTestManager(dc)
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)

	stopCh := make(chan struct{})
	defer close(stopCh)
	unsyncedInformer := cache.NewSharedInformer(&cache.ListWatch{}, &unstructured.Unstructured{}, m.resyncPeriod)
	m.informers.Store(key, info{
		Informer:  unsyncedInformer,
		StopCh:    stopCh,
		Req:       req,
		StartedAt: time.Now(),
	})

	m.monitorInformers()

	informerInfo, ok := loadInfo(t, m, key)
	if !ok {
		t.Fatal("informer removed while within grace period")
	}
	if informerInfo.StopCh != stopCh {
		t.Fatal("informer restarted while within grace period")
	}
}

func TestMonitorRestartsStaleUnsyncedInformer(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	m := newTestManager(dc)
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)
	defer func() { _ = m.StopInformer(key) }()

	stopCh := make(chan struct{})
	unsyncedInformer := cache.NewSharedInformer(&cache.ListWatch{}, &unstructured.Unstructured{}, m.resyncPeriod)
	m.informers.Store(key, info{
		Informer:  unsyncedInformer,
		StopCh:    stopCh,
		Req:       req,
		StartedAt: time.Now().Add(-3 * m.syncGracePeriod),
	})

	m.monitorInformers()

	informerInfo, ok := loadInfo(t, m, key)
	if !ok {
		t.Fatal("informer missing after restart")
	}
	if informerInfo.StopCh == stopCh {
		t.Fatal("informer was not restarted")
	}
	if informerInfo.RestartCount != 1 {
		t.Fatalf("expected RestartCount 1, got %d", informerInfo.RestartCount)
	}
	if !informerInfo.Informer.HasSynced() {
		t.Fatal("restarted informer did not sync")
	}
	select {
	case <-stopCh:
	default:
		t.Fatal("old informer's stop channel was not closed")
	}
}

func TestMonitorStopsInformerForDeletedTarget(t *testing.T) {
	// No objects in the fake client: the target is gone.
	m := newTestManager(newTestDynamicClient())
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)

	stopCh := make(chan struct{})
	unsyncedInformer := cache.NewSharedInformer(&cache.ListWatch{}, &unstructured.Unstructured{}, m.resyncPeriod)
	m.informers.Store(key, info{
		Informer:  unsyncedInformer,
		StopCh:    stopCh,
		Req:       req,
		StartedAt: time.Now().Add(-3 * m.syncGracePeriod),
	})

	m.monitorInformers()

	if _, ok := loadInfo(t, m, key); ok {
		t.Fatal("informer for deleted target was not stopped permanently")
	}
	select {
	case <-stopCh:
	default:
		t.Fatal("stop channel of informer for deleted target was not closed")
	}

	// Subsequent health checks must not resurrect it.
	m.monitorInformers()
	if _, ok := loadInfo(t, m, key); ok {
		t.Fatal("informer for deleted target came back after another health check")
	}
}

func TestMonitorSkipsRestartOnTransientVerifyError(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	dc.PrependReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated API outage")
	})
	m := newTestManager(dc)
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)

	stopCh := make(chan struct{})
	defer close(stopCh)
	unsyncedInformer := cache.NewSharedInformer(&cache.ListWatch{}, &unstructured.Unstructured{}, m.resyncPeriod)
	m.informers.Store(key, info{
		Informer:  unsyncedInformer,
		StopCh:    stopCh,
		Req:       req,
		StartedAt: time.Now().Add(-3 * m.syncGracePeriod),
	})

	m.monitorInformers()

	informerInfo, ok := loadInfo(t, m, key)
	if !ok {
		t.Fatal("informer removed on transient verify error")
	}
	if informerInfo.StopCh != stopCh {
		t.Fatal("informer restarted despite transient verify error")
	}
}

func TestMonitorKeepsRetryingAfterFailedRestart(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	// Target exists (get succeeds), but lists fail so restarts never sync.
	dc.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list failure")
	})
	m := newTestManager(dc)
	m.syncTimeout = 200 * time.Millisecond
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)

	stopCh := make(chan struct{})
	unsyncedInformer := cache.NewSharedInformer(&cache.ListWatch{}, &unstructured.Unstructured{}, m.resyncPeriod)
	m.informers.Store(key, info{
		Informer:  unsyncedInformer,
		StopCh:    stopCh,
		Req:       req,
		StartedAt: time.Now().Add(-3 * m.syncGracePeriod),
	})

	m.monitorInformers()

	informerInfo, ok := loadInfo(t, m, key)
	if !ok {
		t.Fatal("no placeholder stored after failed restart, watch would be lost")
	}
	if informerInfo.StopCh != nil {
		t.Fatal("placeholder after failed restart must not have a running informer")
	}
	if informerInfo.RestartCount != 1 {
		t.Fatalf("expected RestartCount 1, got %d", informerInfo.RestartCount)
	}
	if !informerInfo.NextRestartAt.After(time.Now()) {
		t.Fatal("placeholder must carry a future NextRestartAt for backoff")
	}
	select {
	case <-stopCh:
	default:
		t.Fatal("old informer's stop channel was not closed")
	}
}

func TestConcurrentAddAndStopDoNotOrphanInformers(t *testing.T) {
	dc := newTestDynamicClient(newTestDeployment("demo", "target"))
	m := newTestManager(dc)
	req := newTestRequestWatch("demo", "target")
	key := m.getKeyFromRequestWatch(req)
	defer func() { _ = m.StopInformer(key) }()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = m.Add(req)
		}()
		go func() {
			defer wg.Done()
			_ = m.StopInformer(key)
		}()
	}
	wg.Wait()

	// Whatever the final state, a follow-up Add must leave exactly one running,
	// synced informer for the key.
	if err := m.Add(req); err != nil {
		t.Fatalf("final Add failed: %v", err)
	}
	informerInfo, ok := loadInfo(t, m, key)
	if !ok {
		t.Fatal("informer missing after concurrent add/stop churn")
	}
	if !informerInfo.Informer.HasSynced() {
		t.Fatal("informer not synced after concurrent add/stop churn")
	}
}

func TestRestartBackoffProgression(t *testing.T) {
	m := newTestManager(newTestDynamicClient())
	m.baseRestartBackoff = 5 * time.Second
	m.maxRestartBackoff = 5 * time.Minute

	cases := []struct {
		restartCount int
		want         time.Duration
	}{
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{7, 5 * time.Minute},  // 320s capped
		{20, 5 * time.Minute}, // stays capped
	}
	for _, c := range cases {
		if got := m.restartBackoff(c.restartCount); got != c.want {
			t.Errorf("restartBackoff(%d) = %v, want %v", c.restartCount, got, c.want)
		}
	}
}
