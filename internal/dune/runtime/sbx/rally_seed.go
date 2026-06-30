package sbx

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// rallyHostConfigDir resolves the host rally user-config directory
// ($XDG_CONFIG_HOME/rally or ~/.config/rally). It returns "" when it cannot be
// resolved.
func rallyHostConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return ""
	}
	return filepath.Join(configDir, "rally")
}

// seedHostRallyConfig copies the host's ~/.config/rally into the profile's
// persist directory when that directory's rally config is empty. The sbx
// backend persists to a host directory (mounted into the microVM), so — unlike
// the Compose backend, which copies via a throwaway container over an opaque
// named volume — this is a direct host-side copy. setup-persist.sh later
// symlinks the sandbox's ~/.config/rally at this path and chowns it to the
// agent user, so ownership is normalised in-sandbox regardless of the host
// user's uid.
//
// Returns true when it actually copied host content. Idempotent: a no-op (false,
// nil) once the persist dir already holds any rally config, or when the host
// has nothing to seed.
func seedHostRallyConfig(persistDir string) (bool, error) {
	src := rallyHostConfigDir()
	if src == "" {
		return false, nil
	}
	if entries, err := os.ReadDir(src); err != nil || len(entries) == 0 {
		return false, nil
	}

	dst := filepath.Join(persistDir, ".config", "rally")
	if existing, err := os.ReadDir(dst); err == nil {
		if len(existing) > 0 {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("read persist rally config dir %q: %w", dst, err)
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return false, fmt.Errorf("create persist rally config dir %q: %w", dst, err)
	}
	if err := copyDir(src, dst); err != nil {
		return false, fmt.Errorf("copy rally config %q -> %q: %w", src, dst, err)
	}
	return true, nil
}

// copyDir recursively copies src into dst (which must already exist). File
// modes and symlinks are preserved; existing entries under dst are overwritten.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, d)
	})
}

func copyFile(src, dst string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, lerr := os.Readlink(src)
		if lerr != nil {
			return lerr
		}
		_ = os.Remove(dst)
		return os.Symlink(link, dst)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
