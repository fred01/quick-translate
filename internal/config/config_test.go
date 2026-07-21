package config

import (
	"testing"
	"time"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "valid http with v1", raw: "http://localhost:4000/v1", want: "http://localhost:4000/v1"},
		{name: "valid https with v1", raw: "https://integrate.api.nvidia.com/v1", want: "https://integrate.api.nvidia.com/v1"},
		{name: "trims whitespace", raw: "  http://localhost:4000/v1  ", want: "http://localhost:4000/v1"},
		{name: "trims one trailing slash", raw: "http://localhost:4000/v1/", want: "http://localhost:4000/v1"},
		{name: "does not trim two trailing slashes", raw: "http://localhost:4000/v1//", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
		{name: "whitespace only", raw: "   ", wantErr: true},
		{name: "missing v1", raw: "http://localhost:4000", wantErr: true},
		{name: "missing v1 with trailing slash", raw: "http://localhost:4000/", wantErr: true},
		{name: "invalid scheme", raw: "ftp://localhost:4000/v1", wantErr: true},
		{name: "missing host", raw: "http:///v1", wantErr: true},
		{name: "user info forbidden", raw: "http://user:pass@localhost:4000/v1", wantErr: true},
		{name: "query forbidden", raw: "http://localhost:4000/v1?foo=bar", wantErr: true},
		{name: "fragment forbidden", raw: "http://localhost:4000/v1#frag", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeBaseURL(%q) = %q, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeBaseURL(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestConfigResolve(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid",
			cfg:  Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"},
		},
		{
			name:    "empty model",
			cfg:     Config{BaseURL: "http://localhost:4000/v1", Model: "", APIKey: "secret"},
			wantErr: true,
		},
		{
			name:    "whitespace only model",
			cfg:     Config{BaseURL: "http://localhost:4000/v1", Model: "   ", APIKey: "secret"},
			wantErr: true,
		},
		{
			name:    "empty key",
			cfg:     Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: ""},
			wantErr: true,
		},
		{
			name:    "invalid url",
			cfg:     Config{BaseURL: "not a url", Model: "translategemma", APIKey: "secret"},
			wantErr: true,
		},
		{
			name:    "missing v1 path",
			cfg:     Config{BaseURL: "http://localhost:4000", Model: "translategemma", APIKey: "secret"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.cfg.Resolve()
			if tt.wantErr && err == nil {
				t.Fatalf("Resolve() = nil error, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
		})
	}
}

func TestApplyEnv(t *testing.T) {
	file := Config{BaseURL: "http://file:1/v1", Model: "file-model", APIKey: "file-key"}

	t.Run("env overrides file", func(t *testing.T) {
		env := map[string]string{
			EnvBaseURL: "http://env:2/v1",
			EnvModel:   "env-model",
			EnvAPIKey:  "env-key",
		}
		got := ApplyEnv(file, func(k string) string { return env[k] })
		want := Config{BaseURL: "http://env:2/v1", Model: "env-model", APIKey: "env-key"}
		if got != want {
			t.Fatalf("ApplyEnv() = %+v, want %+v", got, want)
		}
	})

	t.Run("empty env leaves file values", func(t *testing.T) {
		got := ApplyEnv(file, func(string) string { return "" })
		if got != file {
			t.Fatalf("ApplyEnv() = %+v, want %+v", got, file)
		}
	})

	t.Run("partial override", func(t *testing.T) {
		env := map[string]string{EnvModel: "env-model"}
		got := ApplyEnv(file, func(k string) string { return env[k] })
		want := Config{BaseURL: "http://file:1/v1", Model: "env-model", APIKey: "file-key"}
		if got != want {
			t.Fatalf("ApplyEnv() = %+v, want %+v", got, want)
		}
	})

	t.Run("timeout override", func(t *testing.T) {
		env := map[string]string{EnvTimeout: "120"}
		got := ApplyEnv(file, func(k string) string { return env[k] })
		if got.TimeoutSeconds != 120 {
			t.Fatalf("TimeoutSeconds = %d, want 120", got.TimeoutSeconds)
		}
	})

	t.Run("invalid or non-positive timeout is ignored", func(t *testing.T) {
		for _, v := range []string{"abc", "0", "-5", ""} {
			got := ApplyEnv(file, func(k string) string {
				if k == EnvTimeout {
					return v
				}
				return ""
			})
			if got.TimeoutSeconds != 0 {
				t.Fatalf("TimeoutSeconds for %q = %d, want 0 (ignored)", v, got.TimeoutSeconds)
			}
		}
	})
}

func TestConfigTimeout(t *testing.T) {
	if got := (Config{}).Timeout(); got != DefaultTimeout {
		t.Fatalf("unset Timeout() = %v, want DefaultTimeout %v", got, DefaultTimeout)
	}
	if got := (Config{TimeoutSeconds: -1}).Timeout(); got != DefaultTimeout {
		t.Fatalf("negative Timeout() = %v, want DefaultTimeout", got)
	}
	if got := (Config{TimeoutSeconds: 90}).Timeout(); got != 90*time.Second {
		t.Fatalf("Timeout() = %v, want 90s", got)
	}
}
