// Package informer helps you manage your informers/watches and gracefully start and stop them
// It checks health for them, and restrict only 1 informer is running for 1 resource for each crd
package informer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"truefoundry/elasti/operator/internal/prom"

	"github.com/truefoundry/elasti/pkg/config"
	"github.com/truefoundry/elasti/pkg/values"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kRuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/client-go/dynamic"

	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// defaultSyncTimeout bounds how long we block waiting for an informer's initial sync.
	defaultSyncTimeout = 30 * time.Second
	// defaultSyncGracePeriod is how long a freshly (re)started informer gets to finish
	// its initial sync before the health monitor considers restarting it.
	defaultSyncGracePeriod = 2 * time.Minute
	// Backoff bounds for restarting informers that repeatedly fail to sync.
	defaultBaseRestartBackoff = 5 * time.Second
	defaultMaxRestartBackoff  = 5 * time.Minute
	// verifyTargetTimeout bounds the API call checking that a watch target still exists.
	verifyTargetTimeout = 10 * time.Second
)

type (
	// Manager helps manage lifecycle of informer
	Manager struct {
		client        *kubernetes.Clientset
		dynamicClient dynamic.Interface
		logger        *zap.Logger
		informers     sync.Map
		// keyLocks serializes lifecycle operations (start/stop/restart) per informer key,
		// so e.g. Add and the health monitor can't both start an informer for the same key
		// and orphan one of the stop channels.
		keyLocks            sync.Map
		resolver            info
		resyncPeriod        time.Duration
		healthCheckDuration time.Duration
		healthCheckStopChan chan struct{}
		syncTimeout         time.Duration
		syncGracePeriod     time.Duration
		baseRestartBackoff  time.Duration
		maxRestartBackoff   time.Duration
	}

	info struct {
		Informer cache.SharedInformer
		StopCh   chan struct{}
		Req      *RequestWatch
		// StartedAt is when this informer was (re)started.
		StartedAt time.Time
		// RestartCount is how many times the health monitor has restarted this informer.
		RestartCount int
		// NextRestartAt is the earliest time the health monitor may restart this informer.
		NextRestartAt time.Time
	}

	// RequestWatch is the request body sent to the informer
	RequestWatch struct {
		Req                  ctrl.Request
		ResourceName         string
		ResourceNamespace    string
		GroupVersionResource *schema.GroupVersionResource
		Handlers             cache.ResourceEventHandlerFuncs
	}
)

// NewInformerManager creates a new instance of the Informer Manager
func NewInformerManager(logger *zap.Logger, kConfig *rest.Config) *Manager {
	clientSet, err := kubernetes.NewForConfig(kConfig)
	if err != nil {
		logger.Fatal("Error connecting with kubernetes", zap.Error(err))
	}
	dynamicClient, err := dynamic.NewForConfig(kConfig)
	if err != nil {
		logger.Fatal("Error connecting with kubernetes", zap.Error(err))
	}

	return &Manager{
		client:        clientSet,
		dynamicClient: dynamicClient,
		logger:        logger.Named("InformerManager"),
		// ResyncPeriod is the proactive resync we do, even when no events are received by the informer.
		resyncPeriod:        5 * time.Minute,
		healthCheckDuration: 5 * time.Second,
		healthCheckStopChan: make(chan struct{}),
		syncTimeout:         defaultSyncTimeout,
		syncGracePeriod:     defaultSyncGracePeriod,
		baseRestartBackoff:  defaultBaseRestartBackoff,
		maxRestartBackoff:   defaultMaxRestartBackoff,
	}
}

func (m *Manager) InitializeResolverInformer(handlers cache.ResourceEventHandlerFuncs) error {
	deploymentGVR := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	resolverConfig := config.GetResolverConfig()

	m.resolver.Informer = cache.NewSharedInformer(
		&cache.ListWatch{
			ListFunc: func(_ metav1.ListOptions) (kRuntime.Object, error) {
				return m.dynamicClient.Resource(deploymentGVR).Namespace(resolverConfig.Namespace).List(context.Background(), metav1.ListOptions{
					FieldSelector: "metadata.name=" + resolverConfig.DeploymentName,
				})
			},
			WatchFunc: func(_ metav1.ListOptions) (watch.Interface, error) {
				return m.dynamicClient.Resource(deploymentGVR).Namespace(resolverConfig.Namespace).Watch(context.Background(), metav1.ListOptions{
					FieldSelector: "metadata.name=" + resolverConfig.DeploymentName,
				})
			},
		},
		&unstructured.Unstructured{},
		m.resyncPeriod,
	)

	_, err := m.resolver.Informer.AddEventHandler(handlers)
	if err != nil {
		m.logger.Error("Failed to add event handler", zap.Error(err))
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	m.resolver.StopCh = make(chan struct{})
	go m.resolver.Informer.Run(m.resolver.StopCh)

	if !m.waitForSync(m.resolver.StopCh, m.resolver.Informer.HasSynced) {
		close(m.resolver.StopCh)
		m.logger.Error("Failed to sync resolver informer within timeout")
		return errors.New("failed to sync resolver informer")
	}
	m.logger.Info("Resolver informer started")
	return nil
}

// waitForSync waits for the informer cache to sync, bounded by the sync timeout.
// It returns false if the stop channel is closed or the timeout expires first.
func (m *Manager) waitForSync(stopCh <-chan struct{}, hasSynced cache.InformerSynced) bool {
	ctx, cancel := context.WithTimeout(context.Background(), m.syncTimeout)
	defer cancel()
	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return cache.WaitForCacheSync(ctx.Done(), hasSynced)
}

// Start is to initiate a health check on all the running informers
// It uses HasSynced if a informer is not synced, if not, it restarts it
func (m *Manager) Start() {
	m.logger.Info("Starting InformerManager")
	go wait.Until(m.monitorInformers, m.healthCheckDuration, m.healthCheckStopChan)
}

// Stop is to close all the active informers and close the health monitor
func (m *Manager) Stop() {
	m.logger.Info("Stopping InformerManager")
	// Loop through all the informers and stop them
	m.informers.Range(func(_, value interface{}) bool {
		info, ok := value.(info)
		if ok {
			err := m.StopInformer(m.getKeyFromRequestWatch(info.Req))
			if err != nil {
				m.logger.Error("failed to stop informer", zap.Error(err))
			}
		}
		return true
	})
	// Stop the health watch
	close(m.healthCheckStopChan)
	m.logger.Info("InformerManager stopped")
}

// StopForCRD is to close all the active informers for a perticular CRD
func (m *Manager) StopForCRD(crdName string, namespace string) {
	// Loop through all the informers and stop them
	var wg sync.WaitGroup
	m.informers.Range(func(key, value interface{}) bool {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keyStr := key.(string)
			if strings.HasPrefix(keyStr, fmt.Sprintf("%s/%s/", strings.ToLower(crdName), strings.ToLower(namespace))) {
				info, ok := value.(info)
				if ok {
					if err := m.StopInformer(m.getKeyFromRequestWatch(info.Req)); err != nil {
						m.logger.Error("Failed to stop informer", zap.Error(err))
					}
					m.logger.Info("Stopped informer", zap.String("key", keyStr))
				}
			}
		}()
		return true
	})
	wg.Wait()
}

// lockForKey returns the mutex serializing informer lifecycle operations for the given key.
// Lock entries are intentionally never deleted: they are tiny, and deleting them would
// race with concurrent LoadOrStore calls handing out a different mutex for the same key.
func (m *Manager) lockForKey(key string) *sync.Mutex {
	l, _ := m.keyLocks.LoadOrStore(key, &sync.Mutex{})
	return l.(*sync.Mutex)
}

// StopInformer is to stop a informer for a resource
// It closes the shared informer for it and deletes it from the map
func (m *Manager) StopInformer(key string) (err error) {
	defer func() {
		errStr := values.Success
		if err != nil {
			errStr = err.Error()
		}
		prom.InformerCounter.WithLabelValues(key, "stop", errStr).Inc()
	}()
	lock := m.lockForKey(key)
	lock.Lock()
	defer lock.Unlock()
	return m.stopInformerLocked(key)
}

// stopInformerLocked stops the informer for the given key and removes it from the map.
// The caller must hold the key lock.
func (m *Manager) stopInformerLocked(key string) error {
	value, ok := m.informers.Load(key)
	if !ok {
		return fmt.Errorf("informer not found, already stopped for key: %s", key)
	}

	// We need to verify if the informer exists in the map
	informerInfo, ok := value.(info)
	if !ok {
		return fmt.Errorf("failed to cast WatchInfo for key: %s", key)
	}

	// Close the informer, delete it from the map.
	// A nil StopCh means this is a pending-restart placeholder with no running informer.
	if informerInfo.StopCh != nil {
		close(informerInfo.StopCh)
	}
	m.informers.Delete(key)
	prom.InformerGauge.WithLabelValues(key).Dec()
	return nil
}

func (m *Manager) monitorInformers() {
	m.informers.Range(func(key, value interface{}) bool {
		keyStr, keyOk := key.(string)
		informerInfo, ok := value.(info)
		if !keyOk || !ok {
			return true
		}
		if informerInfo.Informer != nil && informerInfo.Informer.HasSynced() {
			if informerInfo.RestartCount != 0 {
				m.resetRestartCount(keyStr)
			}
			return true
		}
		m.checkAndRestartInformer(keyStr)
		return true
	})
}

// resetRestartCount clears the restart backoff state of a now-synced informer,
// so a much later sync problem starts again from the base backoff.
func (m *Manager) resetRestartCount(key string) {
	lock := m.lockForKey(key)
	lock.Lock()
	defer lock.Unlock()

	value, ok := m.informers.Load(key)
	if !ok {
		return
	}
	informerInfo, ok := value.(info)
	if !ok || informerInfo.Informer == nil || !informerInfo.Informer.HasSynced() {
		return
	}
	informerInfo.RestartCount = 0
	informerInfo.NextRestartAt = time.Time{}
	m.informers.Store(key, informerInfo)
}

// checkAndRestartInformer restarts the informer for the given key if it is still
// unsynced, past its sync grace period, past its restart backoff, and its target
// still exists. If the target is gone, the informer is stopped permanently.
func (m *Manager) checkAndRestartInformer(key string) {
	lock := m.lockForKey(key)
	lock.Lock()
	defer lock.Unlock()

	// Re-check under the lock: the informer may have been stopped, restarted or
	// synced since the health check observed it.
	value, ok := m.informers.Load(key)
	if !ok {
		return
	}
	informerInfo, ok := value.(info)
	if !ok {
		return
	}
	if informerInfo.Informer != nil && informerInfo.Informer.HasSynced() {
		return
	}

	now := time.Now()
	// Give a freshly (re)started informer time to complete its initial sync
	// before considering it unhealthy.
	if informerInfo.StopCh != nil && now.Sub(informerInfo.StartedAt) < m.syncGracePeriod {
		return
	}
	// Respect the exponential restart backoff.
	if now.Before(informerInfo.NextRestartAt) {
		return
	}

	// If the watched target (or its namespace) is gone, e.g. a torn-down environment,
	// stop the informer permanently instead of restarting it forever.
	if err := m.verifyTargetExist(informerInfo.Req); err != nil {
		if apierrors.IsNotFound(err) {
			m.logger.Warn("Informer target no longer exists, stopping informer permanently",
				zap.String("key", key), zap.Error(err))
			if stopErr := m.stopInformerLocked(key); stopErr != nil {
				m.logger.Error("Failed to stop informer for missing target", zap.String("key", key), zap.Error(stopErr))
			}
			return
		}
		// Transient error talking to the API server, retry on a later health check.
		m.logger.Error("Failed to verify informer target, skipping restart for now",
			zap.String("key", key), zap.Error(err))
		return
	}

	m.logger.Info("Informer not synced, restarting",
		zap.String("key", key), zap.Int("restartCount", informerInfo.RestartCount+1))
	if err := m.stopInformerLocked(key); err != nil {
		m.logger.Error("Error in stopping informer", zap.String("key", key), zap.Error(err))
	}
	if err := m.enableInformerLocked(informerInfo.Req, informerInfo.RestartCount+1); err != nil {
		m.logger.Error("Error in restarting informer", zap.String("key", key), zap.Error(err))
		// enableInformerLocked cleaned up after itself, so nothing is running for this
		// key anymore. Store a placeholder so later health checks keep retrying with
		// backoff; otherwise the watch would be lost until the operator restarts.
		m.storePlaceholderLocked(key, informerInfo.Req, informerInfo.RestartCount+1)
	}
}

// storePlaceholderLocked records a pending-restart entry (no running informer) so the
// health monitor keeps retrying with backoff. The caller must hold the key lock.
func (m *Manager) storePlaceholderLocked(key string, req *RequestWatch, restartCount int) {
	now := time.Now()
	m.informers.Store(key, info{
		Req:           req,
		StartedAt:     now,
		RestartCount:  restartCount,
		NextRestartAt: now.Add(m.restartBackoff(restartCount)),
	})
	prom.InformerGauge.WithLabelValues(key).Inc()
}

// restartBackoff returns the exponential backoff delay before restart attempt
// number restartCount, capped at maxRestartBackoff.
func (m *Manager) restartBackoff(restartCount int) time.Duration {
	backoff := m.baseRestartBackoff
	for i := 1; i < restartCount; i++ {
		backoff *= 2
		if backoff >= m.maxRestartBackoff {
			return m.maxRestartBackoff
		}
	}
	return backoff
}

// Add is to add a watch on a resource
func (m *Manager) Add(req *RequestWatch) (err error) {
	key := m.getKeyFromRequestWatch(req)
	defer func() {
		errStr := values.Success
		if err != nil {
			errStr = err.Error()
		}
		prom.InformerCounter.WithLabelValues(key, "add", errStr).Inc()
	}()
	m.logger.Info("Adding informer",
		zap.String("group", req.GroupVersionResource.Group),
		zap.String("version", req.GroupVersionResource.Version),
		zap.String("resource", req.GroupVersionResource.Resource),
		zap.String("resourceName", req.ResourceName),
		zap.String("resourceNamespace", req.ResourceNamespace),
		zap.String("crd", req.Req.String()),
	)

	// Serialize with the health monitor and other Add/Stop calls for the same key,
	// so two informers can never run concurrently for one key.
	lock := m.lockForKey(key)
	lock.Lock()
	defer lock.Unlock()

	// Proceed only if the informer is not already running, we verify by checking the map
	if _, ok := m.informers.Load(key); ok {
		m.logger.Info("Informer already running", zap.String("key", key))
		return nil
	}

	if err = m.verifyTargetExist(req); err != nil {
		return fmt.Errorf("target not found: %w", err)
	}

	if err = m.enableInformerLocked(req, 0); err != nil {
		return fmt.Errorf("failed to enable informer: %w", err)
	}
	return nil
}

// enableInformerLocked starts an informer for a resource and stores it in the map.
// It waits up to informerSyncTimeout for the initial sync; on failure the informer
// is stopped and removed again. The caller must hold the key lock.
func (m *Manager) enableInformerLocked(req *RequestWatch, restartCount int) error {
	ctx := context.Background()
	// Create an informer for the resource
	informer := cache.NewSharedInformer(
		&cache.ListWatch{
			ListFunc: func(_ metav1.ListOptions) (kRuntime.Object, error) {
				return m.dynamicClient.Resource(*req.GroupVersionResource).Namespace(req.ResourceNamespace).List(ctx, metav1.ListOptions{
					FieldSelector: "metadata.name=" + req.ResourceName,
				})
			},
			WatchFunc: func(_ metav1.ListOptions) (watch.Interface, error) {
				return m.dynamicClient.Resource(*req.GroupVersionResource).Namespace(req.ResourceNamespace).Watch(ctx, metav1.ListOptions{
					FieldSelector: "metadata.name=" + req.ResourceName,
				})
			},
		},
		&unstructured.Unstructured{},
		m.resyncPeriod,
	)
	// We pass the handlers we received as a parameter
	_, err := informer.AddEventHandler(req.Handlers)
	if err != nil {
		m.logger.Error("Error creating informer handler", zap.Error(err))
		return fmt.Errorf("enableInformer: %w", err)
	}
	// This channel is used to stop the informer
	// We add it in the informers map, so we can stop it when required
	informerStop := make(chan struct{})
	go informer.Run(informerStop)
	// Store the informer in the map
	// This is used to manage the lifecycle of the informer
	// Recover it in case it's not syncing, this is why we also store the handlers
	// Stop it when the CRD or the operator is deleted
	key := m.getKeyFromRequestWatch(req)
	now := time.Now()
	m.informers.Store(key, info{
		Informer:      informer,
		StopCh:        informerStop,
		Req:           req,
		StartedAt:     now,
		RestartCount:  restartCount,
		NextRestartAt: now.Add(m.restartBackoff(restartCount + 1)),
	})
	prom.InformerGauge.WithLabelValues(key).Inc()

	// Wait for the cache to sync, bounded by informerSyncTimeout. On failure, stop the
	// informer and remove it from the map so we don't leak its goroutines.
	if !m.waitForSync(informerStop, informer.HasSynced) {
		m.logger.Error("Failed to sync informer within timeout", zap.String("key", key))
		if stopErr := m.stopInformerLocked(key); stopErr != nil {
			m.logger.Error("Failed to clean up unsynced informer", zap.String("key", key), zap.Error(stopErr))
		}
		return errors.New("failed to sync informer within timeout")
	}
	m.logger.Info("Informer started", zap.String("key", key))
	return nil
}

// getKeyFromRequestWatch is to get the key for the informer map using namespace and resource name from the request
// CRDname.resourcerName.Namespace
func (m *Manager) getKeyFromRequestWatch(req *RequestWatch) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		strings.ToLower(req.Req.Name),                      // CRD Name
		strings.ToLower(req.ResourceNamespace),             // Namespace
		strings.ToLower(req.GroupVersionResource.Resource), // Resource Type
		strings.ToLower(req.ResourceName))                  // Resource Name
}

type KeyParams struct {
	Namespace    string
	CRDName      string
	ResourceType string
	ResourceName string
}

// GetKey is to get the key for the informer map using namespace and resource name
func (m *Manager) GetKey(param KeyParams) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		strings.ToLower(param.CRDName),      // CRD Name
		strings.ToLower(param.Namespace),    // Namespace
		strings.ToLower(param.ResourceType), // Resource Type
		strings.ToLower(param.ResourceName)) // Resource Name
}

// verifyTargetExist is to verify if the target resource exists
func (m *Manager) verifyTargetExist(req *RequestWatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), verifyTargetTimeout)
	defer cancel()
	if _, err := m.dynamicClient.Resource(*req.GroupVersionResource).Namespace(req.ResourceNamespace).Get(ctx, req.ResourceName, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("resource doesn't exist: %w | resource name: %v | resource type: %v | resource namespace: %v", err, req.ResourceName, req.GroupVersionResource.Resource, req.ResourceNamespace)
	}
	return nil
}
