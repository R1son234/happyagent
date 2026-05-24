package career

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const generatedStateVersion = 1

type GeneratedState struct {
	Version int                       `json:"version"`
	Items   []GeneratedArtifactRecord `json:"items"`
}

func (w *Workspace) generatedStatePath() string {
	return filepath.Join(w.Root, WorkspaceInternalDir, "generated_state.json")
}

func (w *Workspace) ReadGeneratedState() (GeneratedState, error) {
	var state GeneratedState
	if err := w.readJSON(w.generatedStatePath(), &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return GeneratedState{Version: generatedStateVersion, Items: []GeneratedArtifactRecord{}}, nil
		}
		return GeneratedState{}, err
	}
	if state.Version == 0 {
		state.Version = generatedStateVersion
	}
	if state.Items == nil {
		state.Items = []GeneratedArtifactRecord{}
	}
	return state, nil
}

func (w *Workspace) WriteGeneratedState(state GeneratedState) error {
	if state.Version == 0 {
		state.Version = generatedStateVersion
	}
	if state.Items == nil {
		state.Items = []GeneratedArtifactRecord{}
	}
	for i, item := range state.Items {
		if err := validateGeneratedArtifactRecord(item); err != nil {
			return fmt.Errorf("generated state item[%d]: %w", i, err)
		}
	}
	return w.writeJSON(w.generatedStatePath(), state)
}

func validateGeneratedArtifactRecord(item GeneratedArtifactRecord) error {
	if err := validateWorkspaceRelPath(item.Path); err != nil {
		return fmt.Errorf("path: %w", err)
	}
	if item.Kind == "" {
		return fmt.Errorf("kind must not be empty")
	}
	for i, ref := range item.SourceRefs {
		if err := validateWorkspaceRelPath(ref.Path); err != nil {
			return fmt.Errorf("source_refs[%d].path: %w", i, err)
		}
	}
	return nil
}
