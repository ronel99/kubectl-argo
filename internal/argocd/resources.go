package argocd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// FetchResourceTree queries Argo CD for the live resource tree of the resolved application.
// It calls GET /api/v1/applications/{resolved-app-name}/resource-tree first, and falls back
// to the gRPC ApplicationService.ResourceTree endpoint if needed.
func (c *Client) FetchResourceTree(ctx context.Context, resolved *ResolvedApp) (*v1alpha1.ApplicationTree, error) {
	if resolved == nil || resolved.AppName == "" {
		return nil, fmt.Errorf("resolved Argo CD application cannot be empty")
	}

	tree, restErr := c.fetchResourceTreeREST(ctx, resolved)
	if restErr == nil {
		return tree, nil
	}

	// Fallback to gRPC ApplicationService.ResourceTree if direct HTTP fails (e.g., custom gRPC proxy setups).
	conn, appClient, err := c.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch resource tree for %q: %w", resolved.AppName, restErr)
	}
	defer func() { _ = conn.Close() }()

	query := &application.ResourcesQuery{
		ApplicationName: &resolved.AppName,
	}
	if resolved.AppNamespace != "" {
		query.AppNamespace = &resolved.AppNamespace
	}

	grpcTree, grpcErr := appClient.ResourceTree(ctx, query)
	if grpcErr != nil {
		return nil, fmt.Errorf("failed to fetch resource tree for application %q (REST: %v, gRPC: %w)", resolved.AppName, restErr, grpcErr)
	}

	return grpcTree, nil
}

// fetchResourceTreeREST performs GET /api/v1/applications/{resolved-app-name}/resource-tree.
func (c *Client) fetchResourceTreeREST(ctx context.Context, resolved *ResolvedApp) (*v1alpha1.ApplicationTree, error) {
	endpoint, err := c.buildRESTURL(fmt.Sprintf("/api/v1/applications/%s/resource-tree", url.PathEscape(resolved.AppName)), resolved.AppNamespace)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct resource-tree request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	req.Header.Set("Accept", "application/json")

	httpClient := c.newHTTPClient(30 * time.Second)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GET %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tree v1alpha1.ApplicationTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("failed to decode resource-tree response: %w", err)
	}

	return &tree, nil
}

// FilterResourceNodes filters an ApplicationTree's nodes by canonical Kubernetes Kind
// (e.g., "Pod", "Service", "Ingress"), target namespace, and optional resource name.
func FilterResourceNodes(tree *v1alpha1.ApplicationTree, kind string, namespace string, resourceName string) []v1alpha1.ResourceNode {
	if tree == nil {
		return nil
	}

	var matched []v1alpha1.ResourceNode
	for _, node := range tree.Nodes {
		if !strings.EqualFold(node.Kind, kind) {
			continue
		}
		if namespace != "" && node.Namespace != "" && !strings.EqualFold(node.Namespace, namespace) {
			continue
		}
		if resourceName != "" && !strings.EqualFold(node.Name, resourceName) {
			continue
		}
		matched = append(matched, node)
	}
	return matched
}

func (c *Client) buildRESTURL(apiPath string, appNamespace string) (string, error) {
	rawServer := strings.TrimSpace(c.cfg.ServerAddr)
	if !strings.HasPrefix(rawServer, "http://") && !strings.HasPrefix(rawServer, "https://") {
		scheme := "https://"
		if c.cfg.PlainText {
			scheme = "http://"
		}
		rawServer = scheme + rawServer
	}

	parsed, err := url.Parse(rawServer)
	if err != nil {
		return "", fmt.Errorf("invalid Argo CD server address %q: %w", c.cfg.ServerAddr, err)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + apiPath
	if appNamespace != "" {
		q := parsed.Query()
		q.Set("appNamespace", appNamespace)
		parsed.RawQuery = q.Encode()
	}

	return parsed.String(), nil
}

func (c *Client) newHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.cfg.Insecure, //nolint:gosec // controlled via explicit --insecure / ARGOCD_INSECURE
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
