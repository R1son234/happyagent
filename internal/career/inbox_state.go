package career

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const inboxStateVersion = 1

type InboxState struct {
	Version int                `json:"version"`
	Items   []PendingInboxItem `json:"items"`
}

func (w *Workspace) inboxStatePath() string {
	return filepath.Join(w.Root, WorkspaceInternalDir, "inbox_state.json")
}

func (w *Workspace) ReadInboxState() (InboxState, error) {
	var state InboxState
	if err := w.readJSON(w.inboxStatePath(), &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InboxState{Version: inboxStateVersion, Items: []PendingInboxItem{}}, nil
		}
		return InboxState{}, err
	}
	if state.Version == 0 {
		state.Version = inboxStateVersion
	}
	if state.Items == nil {
		state.Items = []PendingInboxItem{}
	}
	return state, nil
}

func (w *Workspace) WriteInboxState(state InboxState) error {
	if state.Version == 0 {
		state.Version = inboxStateVersion
	}
	if state.Items == nil {
		state.Items = []PendingInboxItem{}
	}
	for i, item := range state.Items {
		if err := validatePendingInboxItem(item); err != nil {
			return fmt.Errorf("inbox state item[%d]: %w", i, err)
		}
	}
	return w.writeJSON(w.inboxStatePath(), state)
}

func validatePendingInboxItem(item PendingInboxItem) error {
	if item.ID == "" {
		return fmt.Errorf("id must not be empty")
	}
	if err := validateWorkspaceRelPath(item.SourcePath); err != nil {
		return fmt.Errorf("source_path: %w", err)
	}
	if item.ExtractedPath != "" {
		if err := validateWorkspaceRelPath(item.ExtractedPath); err != nil {
			return fmt.Errorf("extracted_path: %w", err)
		}
	}
	switch item.Status {
	case InboxItemStatusPending, InboxItemStatusConfirmed, InboxItemStatusArchived, InboxItemStatusFailed:
	default:
		return fmt.Errorf("unsupported status %q", item.Status)
	}
	return nil
}
