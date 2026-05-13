package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeOffloadObservationWritesLargeOutput(t *testing.T) {
	root := t.TempDir()
	output := strings.Repeat("x", 64)

	result, err := maybeOffloadObservation(OffloadConfig{
		Enabled:  true,
		MinBytes: 32,
		Dir:      ".happyagent/offload",
		RootDir:  root,
		RunID:    "run-1",
	}, "shell", 2, output, "logs/run.log")
	if err != nil {
		t.Fatalf("maybeOffloadObservation() error = %v", err)
	}
	if !result.Offloaded {
		t.Fatalf("expected result to be offloaded")
	}
	if result.Bytes != len(output) {
		t.Fatalf("unexpected bytes: %d", result.Bytes)
	}
	if !strings.Contains(result.Observation, "source: logs/run.log") || !strings.Contains(result.Observation, "Do not repeatedly read this offload path") || !strings.Contains(result.Observation, result.Path) {
		t.Fatalf("unexpected observation: %q", result.Observation)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatalf("read offloaded file: %v", err)
	}
	if string(data) != output {
		t.Fatalf("unexpected offloaded data: %q", string(data))
	}
}

func TestMaybeOffloadObservationSkipsWhenDisabledOrSmall(t *testing.T) {
	result, err := maybeOffloadObservation(OffloadConfig{Enabled: false, MinBytes: 1}, "shell", 1, "large", "")
	if err != nil {
		t.Fatalf("maybeOffloadObservation() error = %v", err)
	}
	if result.Offloaded {
		t.Fatalf("did not expect disabled offload")
	}

	result, err = maybeOffloadObservation(OffloadConfig{Enabled: true, MinBytes: 100, RootDir: t.TempDir()}, "shell", 1, "small", "")
	if err != nil {
		t.Fatalf("maybeOffloadObservation() error = %v", err)
	}
	if result.Offloaded {
		t.Fatalf("did not expect small output to be offloaded")
	}
}

func TestMaybeOffloadObservationRejectsEscapingDir(t *testing.T) {
	_, err := maybeOffloadObservation(OffloadConfig{
		Enabled:  true,
		MinBytes: 1,
		Dir:      "../outside",
		RootDir:  t.TempDir(),
		RunID:    "run-1",
	}, "shell", 1, "large", "")
	if err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
