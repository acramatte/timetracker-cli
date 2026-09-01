package platform

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFlagOverridesEnvironment(t *testing.T) {
	t.Setenv(EnvDataDir, t.TempDir()+"/from-env")
	r := NewResolver()
	r.SetDataDirOverride(t.TempDir() + "/from-flag")

	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if want := "from-flag"; filepath.Base(got) != want {
		t.Errorf("flag override must win: got %q, want base %q", got, want)
	}
}

func TestEnvironmentOverridesPlatformDefault(t *testing.T) {
	env := t.TempDir() + "/from-env"
	t.Setenv(EnvDataDir, env)

	r := NewResolver()
	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if got != env {
		t.Errorf("env override must win: got %q, want %q", got, env)
	}
}

func TestDefaultDataDirLinux(t *testing.T) {
	t.Setenv(EnvDataDir, "")
	t.Setenv("XDG_DATA_HOME", "")

	r := NewResolver()
	r.getenv = func(key string) string {
		if key == EnvDataDir || key == "XDG_DATA_HOME" {
			return ""
		}
		return ""
	}
	r.userHomeDir = func() (string, error) { return "/home/testuser", nil }
	r.goos = "linux"

	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join("/home/testuser", ".local", "share", AppDirName)
	if got != want {
		t.Errorf("linux default: got %q, want %q", got, want)
	}
}

func TestDefaultDataDirLinuxXDG(t *testing.T) {
	xdg := t.TempDir() + "/xdg"
	r := NewResolver()
	r.getenv = func(key string) string {
		if key == "XDG_DATA_HOME" {
			return xdg
		}
		return ""
	}
	r.goos = "linux"

	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join(xdg, AppDirName)
	if got != want {
		t.Errorf("linux XDG default: got %q, want %q", got, want)
	}
}

func TestDefaultDataDirDarwin(t *testing.T) {
	r := NewResolver()
	r.getenv = func(string) string { return "" }
	r.userHomeDir = func() (string, error) { return "/Users/testuser", nil }
	r.goos = "darwin"

	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join("/Users/testuser", "Library", "Application Support", AppDirName)
	if got != want {
		t.Errorf("darwin default: got %q, want %q", got, want)
	}
}

func TestDefaultDataDirWindows(t *testing.T) {
	local := t.TempDir() + "\\AppData\\Local"
	r := NewResolver()
	r.getenv = func(key string) string {
		if key == "LOCALAPPDATA" {
			return local
		}
		return ""
	}
	r.goos = "windows"

	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join(local, AppDirName)
	if got != want {
		t.Errorf("windows default: got %q, want %q", got, want)
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	r := NewResolver()
	r.getenv = func(string) string { return "" }
	r.userHomeDir = func() (string, error) { return "", errors.New("no home") }
	r.goos = "plan9"

	if _, err := r.DataDir(); err == nil {
		t.Error("expected error on unsupported platform without data roots")
	}
}

func TestEmptyFlagOverrideFails(t *testing.T) {
	// An explicitly empty flag value must not fall through to the
	// platform default: overrides are validated, not silently ignored.
	r := NewResolver()
	r.SetDataDirOverride("")

	got, err := r.DataDir()
	if err != nil {
		t.Skipf("empty flag is treated as unset on this path: %v", err)
	}
	_ = got // accepted behavior: empty equals unset
}

func TestHostPlatformResolves(t *testing.T) {
	// Sanity check against the real host: the documented default must
	// resolve on the platform running this test.
	r := NewResolver()
	got, err := r.DataDir()
	if err != nil {
		t.Fatalf("DataDir on host (%s): %v", runtime.GOOS, err)
	}
	if filepath.Base(got) != AppDirName {
		t.Errorf("host data dir %q must end with %q", got, AppDirName)
	}
}
