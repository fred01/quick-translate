// Package config defines the qt configuration schema and validation rules.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Config is the effective qt configuration: the OpenAI-compatible API root,
// the model name, and the API key.
type Config struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

// Environment variable names that override the config file. QT_PROFILE
// selects which profile is active; the others override that profile's
// individual fields.
const (
	EnvBaseURL = "QT_BASE_URL"
	EnvModel   = "QT_MODEL"
	EnvAPIKey  = "QT_API_KEY"
	EnvProfile = "QT_PROFILE"
)

// EnvLookup resolves an environment variable by name, matching os.Getenv.
type EnvLookup func(string) string

// ApplyEnv overlays non-empty environment values onto cfg, giving environment
// variables precedence over the config file. A nil getenv applies nothing.
func ApplyEnv(cfg Config, getenv EnvLookup) Config {
	if getenv == nil {
		return cfg
	}
	if v := getenv(EnvBaseURL); v != "" {
		cfg.BaseURL = v
	}
	if v := getenv(EnvModel); v != "" {
		cfg.Model = v
	}
	if v := getenv(EnvAPIKey); v != "" {
		cfg.APIKey = v
	}
	return cfg
}

// Resolve normalizes the base URL and validates that all three fields are
// present. It returns a copy of cfg with the base URL normalized.
func (c Config) Resolve() (Config, error) {
	base, err := NormalizeBaseURL(c.BaseURL)
	if err != nil {
		return Config{}, err
	}
	c.BaseURL = base
	if strings.TrimSpace(c.Model) == "" {
		return Config{}, errors.New("model must not be empty")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return Config{}, errors.New("API key must not be empty")
	}
	return c, nil
}

// NormalizeBaseURL trims surrounding whitespace and one trailing slash, then
// validates that the result is a usable OpenAI-compatible API root: http(s)
// scheme, a host, no user info, no query, no fragment, and a path ending in
// "/v1". It does not append "/v1" automatically.
func NormalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", errors.New("base URL must not be empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("base URL scheme must be http or https")
	}
	if u.Host == "" {
		return "", errors.New("base URL must include a host")
	}
	if u.User != nil {
		return "", errors.New("base URL must not contain user info")
	}
	if u.RawQuery != "" {
		return "", errors.New("base URL must not contain a query")
	}
	if u.Fragment != "" {
		return "", errors.New("base URL must not contain a fragment")
	}
	if !strings.HasSuffix(u.Path, "/v1") {
		return "", errors.New("base URL must include the /v1 API root")
	}
	return trimmed, nil
}
