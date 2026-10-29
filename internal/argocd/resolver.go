package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
)

const defaultCacheTTL = 5 * time.Minute

// ResolvedApp contains the Argo CD Application metadata mapped from a Kubernetes namespace.
type ResolvedApp struct {
	AppName           string `json:"app_name"`
	AppNamespace      string `json:"app_namespace,omitempty"`
	TargetNamespace   string `json:"target_namespace"`
	DestinationServer string `json:"destination_server,omitempty"`
	DestinationName   string `json:"destination_name,omitempty"`
	Project           string `json:"project,omitempty"`
}

// namespaceCacheFile represents the on-disk JSON cache structure.
type namespaceCacheFile struct {
	ServerAddr string                 `json:"server_addr"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Mappings   map[string]ResolvedApp `json:"mappings"`
}

// Resolver translates a developer-facing Kubernetes namespace (-n <namespace>)
// into its corresponding long Argo CD Application name.
type Resolver struct {
	client       *Client
	cachePath    string
	cacheTTL     time.Duration
	forceRefresh bool

	mu       sync.RWMutex
	mappings map[string]ResolvedApp
}

// ResolverOption configures optional Resolver behavior.
type ResolverOption func(*Resolver)

// WithCacheTTL overrides the default local cache TTL.
func WithCacheTTL(ttl time.Duration) ResolverOption {
	return func(r *Resolver) {
		r.cacheTTL = ttl
	}
}

// WithForceRefresh bypasses any local disk cache and forces a fresh ApplicationService.List call.
func WithForceRefresh(force bool) ResolverOption {
	return func(r *Resolver) {
		r.forceRefresh = force
	}
}

// NewResolver constructs a Resolution Engine backed by the given Argo CD Client.
func NewResolver(client *Client, opts ...ResolverOption) *Resolver {
	r := &Resolver{
		client:    client,
		cachePath: defaultCacheFilePath(),
		cacheTTL:  defaultCacheTTL,
		mappings:  make(map[string]ResolvedApp),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolveNamespace looks up the Argo CD Application name that manages the given Kubernetes namespace.
// It checks the in-memory/disk cache first (unless forceRefresh is enabled), and automatically falls back
// to calling ApplicationService.List on a cache miss.
func (r *Resolver) ResolveNamespace(ctx context.Context, namespace string) (*ResolvedApp, error) {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		return nil, fmt.Errorf("namespace cannot be empty: specify a namespace with -n/--namespace")
	}

	// 1. Check in-memory map first.
	if !r.forceRefresh {
		r.mu.RLock()
		if app, ok := r.mappings[ns]; ok {
			r.mu.RUnlock()
			return &app, nil
		}
		r.mu.RUnlock()

		// 2. Attempt to load from the local disk cache if still within TTL.
		if err := r.loadDiskCache(); err == nil {
			r.mu.RLock()
			if app, ok := r.mappings[ns]; ok {
				r.mu.RUnlock()
				return &app, nil
			}
			r.mu.RUnlock()
		}
	}

	// 3. Cache miss, expired cache, or forced refresh: query Argo CD ApplicationService.List.
	if err := r.Refresh(ctx); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	app, ok := r.mappings[ns]
	if !ok {
		return nil, fmt.Errorf(
			"no Argo CD application found targeting namespace %q (verify the namespace name and your Argo CD RBAC permissions)",
			ns,
		)
	}

	return &app, nil
}

// Refresh fetches all accessible applications via ApplicationService.List,
// builds the spec.destination.namespace -> metadata.name lookup table, and updates the local cache.
func (r *Resolver) Refresh(ctx context.Context) error {
	conn, appClient, err := r.client.NewApplicationClient()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	appList, err := appClient.List(ctx, &application.ApplicationQuery{})
	if err != nil {
		return fmt.Errorf("failed to list Argo CD applications via ApplicationService.List: %w", err)
	}

	newMappings := make(map[string]ResolvedApp, len(appList.Items))
	for _, item := range appList.Items {
		destNS := strings.TrimSpace(item.Spec.Destination.Namespace)
		appName := strings.TrimSpace(item.ObjectMeta.Name)
		if destNS == "" || appName == "" {
			continue
		}

		newMappings[destNS] = ResolvedApp{
			AppName:           appName,
			AppNamespace:      item.ObjectMeta.Namespace,
			TargetNamespace:   destNS,
			DestinationServer: item.Spec.Destination.Server,
			DestinationName:   item.Spec.Destination.Name,
			Project:           item.Spec.Project,
		}
	}

	r.mu.Lock()
	r.mappings = newMappings
	r.mu.Unlock()

	// Best-effort write to disk cache so subsequent CLI invocations are fast.
	_ = r.saveDiskCache(newMappings)

	return nil
}

func (r *Resolver) loadDiskCache() error {
	if r.cachePath == "" {
		return fmt.Errorf("cache path not configured")
	}

	data, err := os.ReadFile(r.cachePath)
	if err != nil {
		return err
	}

	var cached namespaceCacheFile
	if err := json.Unmarshal(data, &cached); err != nil {
		return err
	}

	// Invalidate if server changed or TTL expired.
	if cached.ServerAddr != r.client.Config().ServerAddr {
		return fmt.Errorf("cache server mismatch")
	}
	if time.Since(cached.UpdatedAt) > r.cacheTTL {
		return fmt.Errorf("cache expired")
	}

	r.mu.Lock()
	r.mappings = cached.Mappings
	r.mu.Unlock()
	return nil
}

func (r *Resolver) saveDiskCache(mappings map[string]ResolvedApp) error {
	if r.cachePath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(r.cachePath), 0o700); err != nil {
		return err
	}

	payload := namespaceCacheFile{
		ServerAddr: r.client.Config().ServerAddr,
		UpdatedAt:  time.Now().UTC(),
		Mappings:   mappings,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.cachePath, data, 0o600)
}

func defaultCacheFilePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return ""
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	return filepath.Join(cacheDir, "kubectl-argo", "namespace_cache.json")
}
