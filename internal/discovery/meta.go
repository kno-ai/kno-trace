package discovery

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kno-ai/kno-trace/internal/model"
)

// jsonLine is a minimal struct for extracting timestamp and cwd from any JSONL line.
// We use json.RawMessage-style extraction to avoid parsing full message content.
type jsonLine struct {
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
}

// BuildMeta constructs a SessionMeta from a JSONL file by reading only
// the first and last few lines. This is fast — no full parse needed.
func BuildMeta(filePath string, projectDir string) (*model.SessionMeta, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Extract session ID from filename (UUID.jsonl → UUID).
	base := filepath.Base(filePath)
	id := strings.TrimSuffix(base, ".jsonl")

	meta := &model.SessionMeta{
		ID:            id,
		FilePath:      filePath,
		ProjectDir:    projectDir,
		FileSizeBytes: info.Size(),
	}

	// Read first ~10 lines to find earliest timestamp and cwd.
	// Lines are capped at 10MB each — BuildMeta only needs timestamp/cwd fields
	// which appear early in the JSON, so huge lines are skipped safely.
	const metaMaxLineSize = 10 * 1024 * 1024
	reader := bufio.NewReaderSize(f, 64*1024)
	linesRead := 0
	for linesRead < 10 {
		lineBytes, err := reader.ReadBytes('\n')
		if len(lineBytes) == 0 && err != nil {
			break
		}
		linesRead++
		if len(lineBytes) > metaMaxLineSize {
			if err != nil {
				break
			}
			continue // Skip oversized lines — they won't have useful meta fields.
		}
		var line jsonLine
		if jsonErr := json.Unmarshal(lineBytes, &line); jsonErr != nil {
			if err != nil {
				break
			}
			continue
		}
		if line.Timestamp != "" && meta.StartTime.IsZero() {
			if t, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				meta.StartTime = t
			}
		}
		if line.CWD != "" && meta.ProjectPath == "" {
			meta.ProjectPath = line.CWD
			meta.ProjectName = filepath.Base(line.CWD)
		}
		if !meta.StartTime.IsZero() && meta.ProjectPath != "" {
			break
		}
	}

	// Read last ~10 lines by seeking from end.
	meta.EndTime = readLastTimestamp(f, info.Size())

	if !meta.StartTime.IsZero() && !meta.EndTime.IsZero() {
		meta.Duration = meta.EndTime.Sub(meta.StartTime)
	}

	// Fallback: if we couldn't extract project name from cwd, use directory name.
	if meta.ProjectName == "" {
		meta.ProjectName = projectDir
	}

	return meta, nil
}

// readLastTimestamp reads the last ~10 lines of a file to find the latest timestamp.
// Seeks backward from EOF to find line boundaries.
func readLastTimestamp(f *os.File, size int64) time.Time {
	if size == 0 {
		return time.Time{}
	}

	// Read the last chunk of the file (up to 64KB should cover ~10 lines).
	chunkSize := int64(64 * 1024)
	if chunkSize > size {
		chunkSize = size
	}

	buf := make([]byte, chunkSize)
	_, err := f.ReadAt(buf, size-chunkSize)
	if err != nil && err != io.EOF {
		return time.Time{}
	}

	// Split into lines and process the last ~10.
	content := string(buf)
	lines := strings.Split(content, "\n")

	// Take last 10 non-empty lines.
	var lastLines []string
	for i := len(lines) - 1; i >= 0 && len(lastLines) < 10; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastLines = append(lastLines, lines[i])
		}
	}

	var latest time.Time
	for _, rawLine := range lastLines {
		var line jsonLine
		if err := json.Unmarshal([]byte(rawLine), &line); err != nil {
			continue
		}
		if line.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			continue
		}
		if t.After(latest) {
			latest = t
		}
	}

	return latest
}
