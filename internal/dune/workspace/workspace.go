package workspace

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"claudebox/internal/dune/domain"
)

func Resolve(input string) (domain.WorkspaceRef, error) {
	if input == "" {
		input = "."
	}
	info, err := os.Stat(input)
	if err != nil || !info.IsDir() {
		return domain.WorkspaceRef{}, fmt.Errorf("workspace directory does not exist: %s", input)
	}

	dir, err := filepath.Abs(input)
	if err != nil {
		return domain.WorkspaceRef{}, err
	}

	repoRoot, err := ResolveRepoRoot(dir)
	if err != nil {
		return domain.WorkspaceRef{}, err
	}

	configPath, _ := FindDuneToml(dir)
	if configPath != "" {
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			return domain.WorkspaceRef{}, err
		}
	}

	return domain.WorkspaceRef{
		Input:      input,
		Dir:        dir,
		RepoRoot:   repoRoot,
		ConfigPath: configPath,
		Slug:       slugify(filepath.Base(dir)),
		Hash:       hashDir(dir),
	}, nil
}

func ResolveRepoRoot(directory string) (string, error) {
	cmd := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return filepath.Abs(directory)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return filepath.Abs(directory)
	}
	return filepath.Abs(root)
}

func FindDuneToml(baseDir string) (string, error) {
	if gitRoot, err := ResolveRepoRoot(baseDir); err == nil {
		candidate := filepath.Join(gitRoot, "dune.toml")
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}

	searchDir := baseDir
	for depth := 0; depth <= 5; depth++ {
		candidate := filepath.Join(searchDir, "dune.toml")
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
		if searchDir == "/" {
			break
		}
		searchDir = filepath.Dir(searchDir)
	}

	return "", os.ErrNotExist
}

func ContainerIdentity(ref domain.WorkspaceRef, profile domain.Profile) domain.ContainerIdentity {
	return domain.ContainerIdentity{
		Name:       fmt.Sprintf("dune-%s-%s-%s", ref.Slug, ref.Hash, profile),
		LegacyName: fmt.Sprintf("sand-%s-%s", ref.Slug, ref.Hash),
	}
}

func slugify(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "workspace"
	}
	return result
}

func hashDir(value string) string {
	sum := sha1.Sum([]byte(value))
	return fmt.Sprintf("%x", sum)[:8]
}
