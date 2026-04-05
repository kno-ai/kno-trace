// Package discovery handles finding and listing Claude Code session files.
package discovery

import (
	"os"
	"path/filepath"
	"strings"
)

// EncodePath converts an absolute filesystem path to the directory name
// format used under ~/.claude/projects/. Slashes are replaced with dashes.
// Example: /Users/kevin/code/myproject → -Users-kevin-code-myproject
func EncodePath(absPath string) string {
	return strings.ReplaceAll(absPath, string(os.PathSeparator), "-")
}

// ProjectsDir returns the absolute path to ~/.claude/projects/.
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}
