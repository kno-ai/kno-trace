package discovery

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kno-ai/kno-trace/internal/model"
)

// ScanAll finds all session JSONL files across all projects.
// Returns sessions sorted by modification time descending (most recent first).
func ScanAll() ([]*model.SessionMeta, error) {
	projDir, err := ProjectsDir()
	if err != nil {
		return nil, err
	}
	return scanDir(projDir)
}

// FindCWDSessions finds sessions for the current working directory.
// Encodes the CWD path and looks for a matching project directory.
func FindCWDSessions() ([]*model.SessionMeta, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	projDir, err := ProjectsDir()
	if err != nil {
		return nil, err
	}

	encoded := EncodePath(cwd)

	dirPath := filepath.Join(projDir, encoded)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil, nil // No sessions for this project
	}

	return scanSingleProject(dirPath, encoded)
}

// scanDir scans all project directories under projectsDir.
func scanDir(projectsDir string) ([]*model.SessionMeta, error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var all []*model.SessionMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(projectsDir, entry.Name())
		sessions, err := scanSingleProject(dirPath, entry.Name())
		if err != nil {
			continue // Skip unreadable project directories
		}
		all = append(all, sessions...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].EndTime.After(all[j].EndTime)
	})

	return all, nil
}

// scanSingleProject finds all .jsonl files in a project directory.
func scanSingleProject(dirPath string, projectDir string) ([]*model.SessionMeta, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var sessions []*model.SessionMeta
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		meta, err := BuildMeta(filePath, projectDir)
		if err != nil {
			continue // Skip unreadable session files
		}
		sessions = append(sessions, meta)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].EndTime.After(sessions[j].EndTime)
	})

	return sessions, nil
}
