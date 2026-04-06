package provisioner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// validateName rejects path traversal, D-Bus-illegal path elements,
// and empty/oversized names. Do not "fix up" attacker input.
func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is empty")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("name contains path characters: %q", name)
	}
	if !validName.MatchString(name) {
		return "", fmt.Errorf("name must match %s: %q", validName.String(), name)
	}
	return name, nil
}

// rootedJoin resolves elems under root and rejects any result that
// escapes the root after Clean.
func rootedJoin(root string, elems ...string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)

	parts := append([]string{root}, elems...)
	out := filepath.Clean(filepath.Join(parts...))
	rel, err := filepath.Rel(root, out)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root %s: %s", root, out)
	}
	return out, nil
}
