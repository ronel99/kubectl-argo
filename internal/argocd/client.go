package argocd

import (
	"fmt"
	"io"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"

	"kubectl-argo/internal/config"
)

// Client wraps the Argo CD API client and connection configuration.
type Client struct {
	apiClient apiclient.Client
	cfg       *config.ArgoConfig
}

// NewClient initializes an Argo CD API client from the resolved ArgoConfig.
func NewClient(cfg *config.ArgoConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("argo config cannot be nil")
	}

	opts := &apiclient.ClientOptions{
		ServerAddr: cfg.ServerAddr,
		AuthToken:  cfg.AuthToken,
		Insecure:   cfg.Insecure,
		GRPCWeb:    cfg.GRPCWeb,
		PlainText:  cfg.PlainText,
	}

	rawClient, err := apiclient.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Argo CD API client for %q: %w", cfg.ServerAddr, err)
	}

	return &Client{
		apiClient: rawClient,
		cfg:       cfg,
	}, nil
}

// NewApplicationClient opens a gRPC ApplicationServiceClient connection.
// The caller is responsible for closing the returned io.Closer.
func (c *Client) NewApplicationClient() (io.Closer, application.ApplicationServiceClient, error) {
	conn, appClient, err := c.apiClient.NewApplicationClient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Argo CD ApplicationService client: %w", err)
	}
	return conn, appClient, nil
}

// Config returns the underlying ArgoConfig used by this client.
func (c *Client) Config() *config.ArgoConfig {
	return c.cfg
}
