package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

const (
	dirName  = "qt"
	fileName = "config.json"

	// defaultProfile is the name given to a profile migrated from the old
	// flat single-endpoint config schema.
	defaultProfile = "default"
)

// Store is the on-disk configuration: a set of named profiles and the name
// of the active one. Each profile is a complete endpoint configuration.
type Store struct {
	Active   string            `json:"active"`
	Profiles map[string]Config `json:"profiles"`
}

// Dir returns the directory containing the qt config file, without creating it.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine user config dir: %w", err)
	}
	return filepath.Join(base, dirName), nil
}

// Path returns the full path to the qt config file, without creating it.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// LoadStore reads and parses the config file at path. A missing file is not
// an error: it returns an empty store. A config written in the old flat
// schema ({base_url, model, api_key}) is transparently migrated into a
// profile named "default", marked active.
func LoadStore(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Store{Profiles: map[string]Config{}}, nil
		}
		return Store{}, fmt.Errorf("read config: %w", err)
	}

	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, fmt.Errorf("parse config: %w", err)
	}

	if len(store.Profiles) == 0 {
		// Possibly the old flat schema. Migrate it if it carries any values.
		var flat Config
		if err := json.Unmarshal(data, &flat); err == nil && flat != (Config{}) {
			store = Store{
				Active:   defaultProfile,
				Profiles: map[string]Config{defaultProfile: flat},
			}
		}
	}

	if store.Profiles == nil {
		store.Profiles = map[string]Config{}
	}
	return store, nil
}

// SaveStore writes the store to path atomically: it writes a temporary file
// in the same directory, flushes and closes it, applies restrictive
// permissions on Unix, and renames it over the target. It never leaves a
// partially written config.
func SaveStore(path string, store Store) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	saved := false
	defer func() {
		if !saved {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o600); err != nil {
			return fmt.Errorf("set config file mode: %w", err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	saved = true
	return nil
}

// Names returns the profile names in sorted order.
func (s Store) Names() []string {
	names := make([]string, 0, len(s.Profiles))
	for name := range s.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Upsert adds or replaces the profile named name.
func (s *Store) Upsert(name string, cfg Config) {
	if s.Profiles == nil {
		s.Profiles = map[string]Config{}
	}
	s.Profiles[name] = cfg
}

// SetActive marks name as the active profile. The profile must exist.
func (s *Store) SetActive(name string) error {
	if _, ok := s.Profiles[name]; !ok {
		return fmt.Errorf("no such profile: %q", name)
	}
	s.Active = name
	return nil
}

// Remove deletes the profile named name. It refuses to remove the active
// profile or the last remaining profile.
func (s *Store) Remove(name string) error {
	if _, ok := s.Profiles[name]; !ok {
		return fmt.Errorf("no such profile: %q", name)
	}
	if name == s.Active {
		return fmt.Errorf("cannot remove the active profile %q; switch to another profile first", name)
	}
	if len(s.Profiles) <= 1 {
		return errors.New("cannot remove the only profile")
	}
	delete(s.Profiles, name)
	return nil
}

// chooseProfile resolves which profile name to use, in precedence order:
// the explicit override, then QT_PROFILE, then the stored active profile.
func chooseProfile(store Store, getenv EnvLookup, override string) string {
	if override != "" {
		return override
	}
	if getenv != nil {
		if v := getenv(EnvProfile); v != "" {
			return v
		}
	}
	return store.Active
}

// Effective loads the store at path, selects a profile (override >
// QT_PROFILE > active), overlays the QT_* field environment overrides, and
// validates the result. It returns the resolved configuration and the name
// of the profile that was used (empty when the configuration came purely
// from environment variables). When a profile is explicitly requested (via
// override or QT_PROFILE) but does not exist, it returns an error rather
// than silently falling back.
func Effective(path string, getenv EnvLookup, profileOverride string) (Config, string, error) {
	store, err := LoadStore(path)
	if err != nil {
		return Config{}, "", err
	}

	explicit := profileOverride != ""
	if !explicit && getenv != nil && getenv(EnvProfile) != "" {
		explicit = true
	}
	name := chooseProfile(store, getenv, profileOverride)

	base, ok := store.Profiles[name]
	if !ok {
		if explicit && name != "" && len(store.Profiles) > 0 {
			return Config{}, "", fmt.Errorf("no such profile: %q", name)
		}
		// No matching stored profile; fall back to whatever the environment
		// supplies (env-only configuration). Resolve will report if that is
		// incomplete.
		base = Config{}
		name = ""
	}

	cfg := ApplyEnv(base, getenv)
	resolved, err := cfg.Resolve()
	if err != nil {
		return Config{}, "", err
	}
	return resolved, name, nil
}
