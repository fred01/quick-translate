package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sampleStore() Store {
	return Store{
		Active: "litellm",
		Profiles: map[string]Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "lite-key"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv-key"},
		},
	}
}

func TestPathConstruction(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if !strings.HasSuffix(dir, "qt") {
		t.Fatalf("Dir() = %q, want suffix %q", dir, "qt")
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if filepath.Base(path) != "config.json" {
		t.Fatalf("Path() = %q, want base %q", path, "config.json")
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("Path() dir = %q, want %q", filepath.Dir(path), dir)
	}
}

func TestLoadStoreMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() unexpected error: %v", err)
	}
	if len(store.Profiles) != 0 || store.Active != "" {
		t.Fatalf("LoadStore() = %+v, want empty store", store)
	}
}

func TestSaveAndLoadStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := sampleStore()
	if err := SaveStore(path, want); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}
	got, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if got.Active != want.Active || len(got.Profiles) != len(want.Profiles) {
		t.Fatalf("LoadStore() = %+v, want %+v", got, want)
	}
	for name, cfg := range want.Profiles {
		if got.Profiles[name] != cfg {
			t.Fatalf("profile %q = %+v, want %+v", name, got.Profiles[name], cfg)
		}
	}
}

func TestLoadStoreMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if _, err := LoadStore(path); err == nil {
		t.Fatal("LoadStore() = nil error, want error for malformed JSON")
	}
}

func TestLoadStoreMigratesFlatConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	flat := `{"base_url":"http://localhost:4000/v1","model":"translategemma","api_key":"secret"}`
	if err := os.WriteFile(path, []byte(flat), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	store, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if store.Active != "default" {
		t.Fatalf("migrated active = %q, want default", store.Active)
	}
	got, ok := store.Profiles["default"]
	if !ok {
		t.Fatal("migrated store missing default profile")
	}
	want := Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"}
	if got != want {
		t.Fatalf("migrated default profile = %+v, want %+v", got, want)
	}
}

func TestEffectiveActiveProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	cfg, name, err := Effective(path, func(string) string { return "" }, "")
	if err != nil {
		t.Fatalf("Effective() error: %v", err)
	}
	if name != "litellm" {
		t.Fatalf("chosen profile = %q, want litellm (active)", name)
	}
	if cfg.Model != "translategemma" {
		t.Fatalf("cfg.Model = %q", cfg.Model)
	}
}

func TestEffectiveProfileOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	cfg, name, err := Effective(path, func(string) string { return "" }, "nvidia")
	if err != nil {
		t.Fatalf("Effective() error: %v", err)
	}
	if name != "nvidia" {
		t.Fatalf("chosen profile = %q, want nvidia (override)", name)
	}
	if cfg.Model != "llama" {
		t.Fatalf("cfg.Model = %q, want llama", cfg.Model)
	}
}

func TestEffectiveProfilePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	env := map[string]string{EnvProfile: "nvidia"}
	getenv := func(k string) string { return env[k] }

	// QT_PROFILE selects nvidia over the stored active litellm.
	_, name, err := Effective(path, getenv, "")
	if err != nil {
		t.Fatalf("Effective() error: %v", err)
	}
	if name != "nvidia" {
		t.Fatalf("QT_PROFILE precedence: chosen = %q, want nvidia", name)
	}

	// The explicit override beats QT_PROFILE.
	_, name, err = Effective(path, getenv, "litellm")
	if err != nil {
		t.Fatalf("Effective() error: %v", err)
	}
	if name != "litellm" {
		t.Fatalf("override precedence: chosen = %q, want litellm", name)
	}
}

func TestEffectiveFieldEnvOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	env := map[string]string{EnvModel: "env-model"}
	cfg, _, err := Effective(path, func(k string) string { return env[k] }, "")
	if err != nil {
		t.Fatalf("Effective() error: %v", err)
	}
	if cfg.Model != "env-model" {
		t.Fatalf("cfg.Model = %q, want env-model (field override)", cfg.Model)
	}
}

func TestEffectiveUnknownProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}
	if _, _, err := Effective(path, func(string) string { return "" }, "nope"); err == nil {
		t.Fatal("Effective() = nil error, want error for unknown profile")
	}
}

func TestEffectiveNoProfilesIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if _, _, err := Effective(path, func(string) string { return "" }, ""); err == nil {
		t.Fatal("Effective() = nil error, want error when nothing is configured")
	}
}

func TestStoreOps(t *testing.T) {
	s := sampleStore()

	if got := s.Names(); len(got) != 2 || got[0] != "litellm" || got[1] != "nvidia" {
		t.Fatalf("Names() = %v, want sorted [litellm nvidia]", got)
	}

	s.Upsert("local", Config{BaseURL: "http://localhost:1/v1", Model: "m", APIKey: "k"})
	if _, ok := s.Profiles["local"]; !ok {
		t.Fatal("Upsert did not add the profile")
	}

	if err := s.SetActive("nvidia"); err != nil {
		t.Fatalf("SetActive() error: %v", err)
	}
	if s.Active != "nvidia" {
		t.Fatalf("Active = %q, want nvidia", s.Active)
	}
	if err := s.SetActive("ghost"); err == nil {
		t.Fatal("SetActive(ghost) = nil error, want error")
	}

	// Cannot remove the active profile.
	if err := s.Remove("nvidia"); err == nil {
		t.Fatal("Remove(active) = nil error, want error")
	}
	// Can remove a non-active profile.
	if err := s.Remove("litellm"); err != nil {
		t.Fatalf("Remove(litellm) error: %v", err)
	}
	if _, ok := s.Profiles["litellm"]; ok {
		t.Fatal("Remove did not delete the profile")
	}
}

func TestRemoveLastProfileIsError(t *testing.T) {
	s := Store{Active: "only", Profiles: map[string]Config{"only": {BaseURL: "http://x/v1", Model: "m", APIKey: "k"}}}
	// Switch active away is impossible (only one), so removing the only
	// profile must fail on both guards; assert it fails.
	if err := s.Remove("only"); err == nil {
		t.Fatal("Remove of the only profile must fail")
	}
}

func TestSaveStoreAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}
	second := Store{Active: "only", Profiles: map[string]Config{"only": {BaseURL: "http://second/v1", Model: "m2", APIKey: "k2"}}}
	if err := SaveStore(path, second); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	got, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if got.Active != "only" || len(got.Profiles) != 1 {
		t.Fatalf("LoadStore() = %+v, want the second store", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("unexpected file left behind: %q", e.Name())
		}
	}
}

func TestSaveStoreFailureDoesNotCorruptExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based failure injection is unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := sampleStore()
	if err := SaveStore(path, original); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod() error: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	broken := Store{Active: "x", Profiles: map[string]Config{"x": {BaseURL: "http://broken/v1", Model: "m", APIKey: "k"}}}
	if err := SaveStore(path, broken); err == nil {
		t.Fatal("SaveStore() = nil error, want error when directory is read-only")
	}
	os.Chmod(dir, 0o700)

	got, err := LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if got.Active != original.Active {
		t.Fatalf("active = %q, want unchanged %q", got.Active, original.Active)
	}
}

func TestSaveStoreUnixModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode assertions are unix-only")
	}
	root := t.TempDir()
	path := filepath.Join(root, "qt-config-dir", "config.json")
	if err := SaveStore(path, sampleStore()); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %v, want 0600", got)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat() dir error: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", got)
	}
}

// TestStoreJSONShape guards the on-disk schema shape.
func TestStoreJSONShape(t *testing.T) {
	data, err := json.MarshalIndent(sampleStore(), "", "  ")
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"active"`, `"profiles"`, `"base_url"`, `"model"`, `"api_key"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("schema missing %s:\n%s", want, s)
		}
	}
}
