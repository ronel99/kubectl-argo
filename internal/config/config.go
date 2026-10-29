package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ArgoConfig holds the resolved connection and authentication settings for Argo CD.
type ArgoConfig struct {
	ServerAddr string
	AuthToken  string
	Insecure   bool
	GRPCWeb    bool
	PlainText  bool
	ConfigPath string
}

// argoLocalContext represents an entry under "contexts" in ~/.argocd/config.
type argoLocalContext struct {
	Name   string `mapstructure:"name"`
	Server string `mapstructure:"server"`
	User   string `mapstructure:"user"`
}

// argoLocalServer represents an entry under "servers" in ~/.argocd/config.
type argoLocalServer struct {
	Server          string `mapstructure:"server"`
	PlainText       bool   `mapstructure:"plain-text"`
	Insecure        bool   `mapstructure:"insecure"`
	GRPCWeb         bool   `mapstructure:"grpc-web"`
	GRPCWebRootPath string `mapstructure:"grpc-web-root-path"`
}

// argoLocalUser represents an entry under "users" in ~/.argocd/config.
type argoLocalUser struct {
	Name      string `mapstructure:"name"`
	AuthToken string `mapstructure:"auth-token"`
}

// argoLocalConfig mirrors the standard ~/.argocd/config structure written by `argocd login --sso`.
type argoLocalConfig struct {
	CurrentContext string             `mapstructure:"current-context"`
	Contexts       []argoLocalContext `mapstructure:"contexts"`
	Servers        []argoLocalServer  `mapstructure:"servers"`
	Users          []argoLocalUser    `mapstructure:"users"`
}

// Load reads Argo CD connection settings using Viper.
// Precedence (highest to lowest):
//  1. Explicit CLI flags bound to Viper (server, auth-token, insecure, grpc-web, plaintext)
//  2. Environment variables (ARGOCD_SERVER, ARGOCD_AUTH_TOKEN, ARGOCD_INSECURE, ARGOCD_GRPC_WEB, ARGOCD_PLAINTEXT)
//  3. Standard Argo CD configuration file (~/.argocd/config)
func Load(customConfigPath string) (*ArgoConfig, error) {
	v := viper.GetViper()

	// Bind standard Argo CD environment variables.
	_ = v.BindEnv("server", "ARGOCD_SERVER")
	_ = v.BindEnv("auth-token", "ARGOCD_AUTH_TOKEN")
	_ = v.BindEnv("insecure", "ARGOCD_INSECURE")
	_ = v.BindEnv("grpc-web", "ARGOCD_GRPC_WEB")
	_ = v.BindEnv("plaintext", "ARGOCD_PLAINTEXT")

	// Resolve config file path (~/.argocd/config by default).
	cfgPath := customConfigPath
	if cfgPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			cfgPath = filepath.Join(homeDir, ".argocd", "config")
		}
	}

	if cfgPath != "" {
		if _, err := os.Stat(cfgPath); err == nil {
			v.SetConfigFile(cfgPath)
			v.SetConfigType("yaml")
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("failed to parse Argo CD config file %q: %w", cfgPath, err)
			}
		}
	}

	cfg := &ArgoConfig{
		ConfigPath: cfgPath,
	}

	// First, extract settings from the native ~/.argocd/config multi-context structure if present.
	var localCfg argoLocalConfig
	if err := v.Unmarshal(&localCfg); err == nil && localCfg.CurrentContext != "" {
		applyContextConfig(cfg, &localCfg)
	}

	// Override with top-level keys, environment variables, or CLI flags when set.
	if s := strings.TrimSpace(v.GetString("server")); s != "" {
		cfg.ServerAddr = s
	}
	if t := strings.TrimSpace(v.GetString("auth-token")); t != "" {
		cfg.AuthToken = t
	}
	if v.IsSet("insecure") {
		cfg.Insecure = v.GetBool("insecure")
	}
	if v.IsSet("grpc-web") {
		cfg.GRPCWeb = v.GetBool("grpc-web")
	}
	if v.IsSet("plaintext") {
		cfg.PlainText = v.GetBool("plaintext")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyContextConfig resolves the active context's server and user entries from ~/.argocd/config.
func applyContextConfig(dst *ArgoConfig, raw *argoLocalConfig) {
	var activeCtx *argoLocalContext
	for i := range raw.Contexts {
		if raw.Contexts[i].Name == raw.CurrentContext {
			activeCtx = &raw.Contexts[i]
			break
		}
	}
	if activeCtx == nil {
		return
	}

	for _, srv := range raw.Servers {
		if srv.Server == activeCtx.Server {
			dst.ServerAddr = srv.Server
			dst.Insecure = srv.Insecure
			dst.GRPCWeb = srv.GRPCWeb
			dst.PlainText = srv.PlainText
			break
		}
	}
	if dst.ServerAddr == "" {
		dst.ServerAddr = activeCtx.Server
	}

	for _, usr := range raw.Users {
		if usr.Name == activeCtx.User {
			dst.AuthToken = usr.AuthToken
			break
		}
	}
}

// Validate ensures mandatory Argo CD connection parameters are available.
func (c *ArgoConfig) Validate() error {
	if c.ServerAddr == "" {
		return errors.New("Argo CD server address not configured: run 'argocd login <server> --sso' or set ARGOCD_SERVER")
	}
	if c.AuthToken == "" {
		return errors.New("Argo CD auth token not found: run 'argocd login <server> --sso' or set ARGOCD_AUTH_TOKEN")
	}
	return nil
}
