package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultOffloadMinBytes = 12 * 1024

type offloadResult struct {
	Offloaded   bool
	Observation string
	Path        string
	Bytes       int
}

func maybeOffloadObservation(config OffloadConfig, toolName string, stepIndex int, output string) (offloadResult, error) {
	if !config.Enabled {
		return offloadResult{}, nil
	}
	minBytes := config.MinBytes
	if minBytes <= 0 {
		minBytes = defaultOffloadMinBytes
	}
	if len(output) < minBytes {
		return offloadResult{}, nil
	}

	root := strings.TrimSpace(config.RootDir)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return offloadResult{}, fmt.Errorf("resolve offload root: %w", err)
	}

	dir := strings.TrimSpace(config.Dir)
	if dir == "" {
		dir = ".happyagent/offload"
	}
	runID := sanitizePathSegment(config.RunID)
	if runID == "" {
		runID = "run"
	}

	relativeDir := filepath.Clean(dir)
	if filepath.IsAbs(relativeDir) {
		relativeDir, err = filepath.Rel(absRoot, relativeDir)
		if err != nil {
			return offloadResult{}, fmt.Errorf("make offload dir relative to root: %w", err)
		}
	}
	if strings.HasPrefix(relativeDir, ".."+string(filepath.Separator)) || relativeDir == ".." {
		return offloadResult{}, fmt.Errorf("offload dir %q escapes root %q", dir, absRoot)
	}

	hash := sha256.Sum256([]byte(output))
	name := fmt.Sprintf("step-%d-%s-%s.txt", stepIndex, sanitizePathSegment(toolName), hex.EncodeToString(hash[:])[:12])
	relativePath := filepath.Join(relativeDir, runID, name)
	absPath := filepath.Join(absRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return offloadResult{}, fmt.Errorf("create offload dir: %w", err)
	}
	if err := os.WriteFile(absPath, []byte(output), 0o600); err != nil {
		return offloadResult{}, fmt.Errorf("write offload result: %w", err)
	}

	displayPath := filepath.ToSlash(relativePath)
	return offloadResult{
		Offloaded: true,
		Observation: fmt.Sprintf("[offloaded tool result]\ntool: %s\nbytes: %d\npath: %s\nUse file_read with this path to inspect the saved output.",
			toolName, len(output), displayPath),
		Path:  displayPath,
		Bytes: len(output),
	}, nil
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), ".-")
}
