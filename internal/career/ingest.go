package career

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type IngestRequest struct {
	Path      string
	HintType  string
	UserInput string
	Now       time.Time
}

type IngestResult struct {
	Item         WorkspaceItem
	OriginalRel  string
	ExtractedRel string
	ItemType     string
}

type InboxIngestResult struct {
	Items    []WorkspaceItem
	Warnings []string
}

func IngestFile(ctx context.Context, ws *Workspace, req IngestRequest) (IngestResult, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return IngestResult{}, fmt.Errorf("ingest path must not be empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return IngestResult{}, fmt.Errorf("resolve %q: %w", path, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return IngestResult{}, fmt.Errorf("stat %q: %w", absPath, err)
	}
	if info.IsDir() {
		return IngestResult{}, fmt.Errorf("%q is a directory, expected a file", absPath)
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	guide, err := ws.LoadGuide()
	if err != nil {
		return IngestResult{}, err
	}
	extracted, err := extractDocument(ctx, absPath)
	if err != nil {
		ext := strings.ToLower(filepath.Ext(absPath))
		extractor, mimeType := extractorInfoForExt(ext)
		classification := inferIngestClassification(guide, req.HintType, absPath, req.UserInput, "")
		itemType := classification.Type
		if !IsSupportedWorkspaceType(itemType) {
			itemType = WorkspaceTypeRecord
		}
		result, archiveErr := ws.AddGuidedMaterial(GuidedMaterialInput{
			ItemType:       itemType,
			Classification: classification,
			SourceLabel:    absPath,
			Now:            now,
			File: WorkspaceFileInput{
				ItemType:      itemType,
				OriginalPath:  absPath,
				OriginalName:  filepath.Base(absPath),
				Now:           now,
				Extractor:     extractor,
				MIMEType:      mimeType,
				ExtractStatus: "failed",
				ExtractError:  err.Error(),
			},
		})
		if archiveErr != nil {
			return IngestResult{}, archiveErr
		}
		return IngestResult{
			Item:        result.Item,
			OriginalRel: result.Item.Metadata.Original,
			ItemType:    result.Item.Type,
		}, err
	}
	classification := inferIngestClassification(guide, req.HintType, absPath, req.UserInput, extracted.Text)
	itemType := classification.Type
	if !IsSupportedWorkspaceType(itemType) {
		return IngestResult{}, fmt.Errorf("unable to classify referenced file %q", absPath)
	}
	if itemType == WorkspaceTypeGeneral || !classification.ShouldSave {
		itemType = WorkspaceTypeRecord
		classification.Type = WorkspaceTypeRecord
		if classification.Reason == "" || classification.Type == WorkspaceTypeGeneral {
			classification.Reason = "low confidence classification saved as record"
		}
	}
	result, err := ws.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       itemType,
		Classification: classification,
		SourceLabel:    absPath,
		Now:            now,
		File: WorkspaceFileInput{
			ItemType:      itemType,
			Text:          extracted.Text,
			OriginalPath:  absPath,
			OriginalName:  filepath.Base(absPath),
			Now:           now,
			Extractor:     extracted.Extractor,
			MIMEType:      extracted.MIMEType,
			ExtractStatus: extracted.ExtractStatus,
			ExtractError:  extracted.ExtractError,
		},
	})
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		Item:         result.Item,
		OriginalRel:  result.Item.Metadata.Original,
		ExtractedRel: result.Item.Metadata.Source,
		ItemType:     result.Item.Type,
	}, nil
}

func DiscoverInboxFiles(workspace *Workspace) ([]string, error) {
	if workspace == nil {
		return nil, fmt.Errorf("workspace must not be nil")
	}
	inboxRoot := filepath.Join(workspace.Root, "inbox")
	entries, err := os.ReadDir(inboxRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read inbox: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		paths = append(paths, filepath.Join(inboxRoot, entry.Name()))
	}
	return paths, nil
}

func IngestInbox(ctx context.Context, workspace *Workspace, now time.Time) (InboxIngestResult, error) {
	paths, err := DiscoverInboxFiles(workspace)
	if err != nil {
		return InboxIngestResult{}, err
	}
	result := InboxIngestResult{}
	for _, path := range paths {
		ingested, ingestErr := IngestFile(ctx, workspace, IngestRequest{
			Path:      path,
			UserInput: filepath.Base(path),
			Now:       now,
		})
		if ingestErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", path, ingestErr))
			continue
		}
		result.Items = append(result.Items, ingested.Item)
	}
	return result, nil
}

func inferIngestItemType(hintType string, path string, userInput string, content string) string {
	classification := inferIngestClassification(DefaultWorkspaceGuide(), hintType, path, userInput, content)
	if IsSupportedWorkspaceType(classification.Type) {
		return classification.Type
	}
	return ""
}

func inferIngestClassification(guide WorkspaceGuide, hintType string, path string, userInput string, content string) InputClassification {
	nearHint := ""
	if hinted := detectWorkspaceTypeHintNearPathWithGuide(userInput, path, guide); hinted != "" {
		nearHint = hinted
	}
	if nearHint != "" && strings.TrimSpace(hintType) == "" {
		hintType = nearHint
	}
	return ClassifyInputWithSignals(content, guide, filepath.Base(path), hintType, "")
}
