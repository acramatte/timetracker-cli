// Package platform resolves user-local storage locations for the
// timetracker CLI across Linux, macOS, and Windows.
//
// Precedence for the data directory (spec 001-local-first-cli §8.2):
//  1. --data-dir flag (set via SetDataDirOverride)
//  2. TIMETRACKER_DATA_DIR environment variable
//  3. Platform default user-data location
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// EnvDataDir is the environment variable overriding the data directory.
const EnvDataDir = "TIMETRACKER_DATA_DIR"

// AppDirName is the directory name used beneath platform data roots.
const AppDirName = "timetracker"

// errOverrideInvalid reports an empty explicit override.
var errOverrideInvalid = errors.New("data directory override is empty")

// Resolver determines the data directory for a run of the CLI.
type Resolver struct {
	// flagOverride holds an explicit --data-dir value; it wins over env.
	flagOverride string
	// getenv is swappable for tests.
	getenv func(string) string
	// userHomeDir is swappable for tests.
	userHomeDir func() (string, error)
	// goos is swappable for tests.
	goos string
}

// NewResolver returns a resolver that reads the real environment.
func NewResolver() *Resolver {
	return &Resolver{
		getenv:      os.Getenv,
		userHomeDir: os.UserHomeDir,
		goos:        runtime.GOOS,
	}
}

// SetDataDirOverride records the --data-dir flag value.
// The flag takes precedence over the environment variable.
func (r *Resolver) SetDataDirOverride(dir string) {
	r.flagOverride = dir
}

// DataDir returns the resolved data directory.
func (r *Resolver) DataDir() (string, error) {
	if r.flagOverride != "" {
		if err := validDir(r.flagOverride); err != nil {
			return "", fmt.Errorf("--data-dir: %w", err)
		}
		return r.flagOverride, nil
	}
	if env := r.getenv(EnvDataDir); env != "" {
		if err := validDir(env); err != nil {
			return "", fmt.Errorf("%s: %w", EnvDataDir, err)
		}
		return env, nil
	}
	return r.defaultDataDir()
}

// defaultDataDir mirrors the platform table in spec §8.1.
func (r *Resolver) defaultDataDir() (string, error) {
	switch r.goos {
	case "windows":
		if appdata := r.getenv("LOCALAPPDATA"); appdata != "" {
			return filepath.Join(appdata, AppDirName), nil
		}
	case "darwin":
		home, err := r.userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", AppDirName), nil
	default: // linux and other unix-like
		if xdg := r.getenv("XDG_DATA_HOME"); xdg != "" {
			if err := validDir(xdg); err != nil {
				return "", fmt.Errorf("XDG_DATA_HOME: %w", err)
			}
			return filepath.Join(xdg, AppDirName), nil
		}
		home, err := r.userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, ".local", "share", AppDirName), nil
	}
	return "", fmt.Errorf("unsupported platform %q with no resolvable data root", r.goos)
}

func validDir(dir string) error {
	if dir == "" {
		return errOverrideInvalid
	}
	return nil
}
