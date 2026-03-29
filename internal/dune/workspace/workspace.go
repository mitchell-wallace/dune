package workspace

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Ref struct {
	Input string
	Root  string
	Slug  string
}

func Resolve(input string) (Ref, error) {
	if strings.TrimSpace(input) == "" {
		input = "."
	}

	info, err := os.Stat(input)
	if err != nil {
		return Ref{}, fmt.Errorf("workspace path %q does not exist", input)
	}
	if !info.IsDir() {
		return Ref{}, fmt.Errorf("workspace path %q is not a directory", input)
	}

	absInput, err := filepath.Abs(input)
	if err != nil {
		return Ref{}, fmt.Errorf("resolve workspace path: %w", err)
	}

	root, err := ResolveRoot(absInput)
	if err != nil {
		return Ref{}, err
	}

	return Ref{
		Input: input,
		Root:  root,
		Slug:  Slug(root),
	}, nil
}

func ResolveRoot(directory string) (string, error) {
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	cmd := exec.Command("git", "-C", absDir, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return absDir, nil
	}

	root := strings.TrimSpace(string(output))
	if root == "" {
		return absDir, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve git root: %w", err)
	}
	return absRoot, nil
}

func Slug(root string) string {
	base := sanitize(filepath.Base(root))
	if base == "" {
		base = "workspace"
	}
	sum := sha1.Sum([]byte(root))
	return fmt.Sprintf("%s-%x", base, sum[:1])
}

func sanitize(value string) string {
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

	return strings.Trim(builder.String(), "-")
}
