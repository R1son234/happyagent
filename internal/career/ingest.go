package career

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

type ExtractedDocument struct {
	Text          string
	Extractor     string
	MIMEType      string
	ExtractStatus string
	ExtractError  string
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
	extracted, err := extractDocument(ctx, absPath)
	if err != nil {
		ext := strings.ToLower(filepath.Ext(absPath))
		extractor, mimeType := extractorInfoForExt(ext)
		itemType := inferIngestItemType(req.HintType, absPath, req.UserInput, "")
		if !IsSupportedWorkspaceType(itemType) {
			itemType = WorkspaceTypeGeneral
		}
		item, archiveErr := ws.AddMaterialFromFile(WorkspaceFileInput{
			ItemType:      itemType,
			OriginalPath:  absPath,
			OriginalName:  filepath.Base(absPath),
			Now:           now,
			Extractor:     extractor,
			MIMEType:      mimeType,
			ExtractStatus: "failed",
			ExtractError:  err.Error(),
		})
		if archiveErr != nil {
			return IngestResult{}, archiveErr
		}
		return IngestResult{
			Item:        item,
			OriginalRel: item.Metadata.Original,
			ItemType:    item.Type,
		}, err
	}
	itemType := inferIngestItemType(req.HintType, absPath, req.UserInput, extracted.Text)
	if !IsSupportedWorkspaceType(itemType) {
		return IngestResult{}, fmt.Errorf("unable to classify referenced file %q", absPath)
	}
	item, err := ws.AddMaterialFromFile(WorkspaceFileInput{
		ItemType:      itemType,
		Text:          extracted.Text,
		OriginalPath:  absPath,
		OriginalName:  filepath.Base(absPath),
		Now:           now,
		Extractor:     extracted.Extractor,
		MIMEType:      extracted.MIMEType,
		ExtractStatus: extracted.ExtractStatus,
		ExtractError:  extracted.ExtractError,
	})
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		Item:         item,
		OriginalRel:  item.Metadata.Original,
		ExtractedRel: item.Metadata.Source,
		ItemType:     item.Type,
	}, nil
}

func inferIngestItemType(hintType string, path string, userInput string, content string) string {
	if IsSupportedWorkspaceType(hintType) {
		return hintType
	}
	if hinted := detectWorkspaceTypeHintNearPath(userInput, path); hinted != "" {
		return hinted
	}
	nameHint := detectWorkspaceTypeHint(filepath.Base(path))
	if nameHint != "" {
		return nameHint
	}
	classification := ClassifyInput(content)
	if IsSupportedWorkspaceType(classification.Type) {
		return classification.Type
	}
	return ""
}

func extractDocument(ctx context.Context, path string) (ExtractedDocument, error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".txt", ".md":
		data, err := os.ReadFile(path)
		if err != nil {
			return ExtractedDocument{}, fmt.Errorf("read %q: %w", path, err)
		}
		return ExtractedDocument{
			Text:          normalizeExtractedText(string(data)),
			Extractor:     "plain_text",
			MIMEType:      mimeTypeForExt(ext),
			ExtractStatus: "ok",
		}, nil
	case ".docx":
		text, err := extractDOCXText(path)
		if err != nil {
			return ExtractedDocument{}, err
		}
		return ExtractedDocument{
			Text:          normalizeExtractedText(text),
			Extractor:     "documents",
			MIMEType:      mimeTypeForExt(ext),
			ExtractStatus: "ok",
		}, nil
	case ".pdf":
		text, err := extractPDFText(ctx, path)
		if err != nil {
			return ExtractedDocument{}, err
		}
		return ExtractedDocument{
			Text:          normalizeExtractedText(text),
			Extractor:     "pdf",
			MIMEType:      mimeTypeForExt(ext),
			ExtractStatus: "ok",
		}, nil
	default:
		return ExtractedDocument{}, fmt.Errorf("unsupported file type %q", ext)
	}
}

func extractorInfoForExt(ext string) (string, string) {
	switch ext {
	case ".docx":
		return "documents", mimeTypeForExt(ext)
	case ".pdf":
		return "pdf", mimeTypeForExt(ext)
	case ".md", ".txt":
		return "plain_text", mimeTypeForExt(ext)
	default:
		return "unknown", "application/octet-stream"
	}
}

func mimeTypeForExt(ext string) string {
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func normalizeExtractedText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	var out []string
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func extractDOCXText(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open DOCX %q: %w", path, err)
	}
	defer reader.Close()

	documentFile, err := findZipFile(reader.File, "word/document.xml")
	if err != nil {
		return "", err
	}
	rc, err := documentFile.Open()
	if err != nil {
		return "", fmt.Errorf("open word/document.xml: %w", err)
	}
	defer rc.Close()

	text, err := parseWordprocessingML(rc)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("DOCX %q did not contain extractable text", path)
	}
	return text, nil
}

func findZipFile(files []*zip.File, name string) (*zip.File, error) {
	for _, file := range files {
		if file.Name == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("missing %s in DOCX archive", name)
}

func parseWordprocessingML(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("parse DOCX xml: %w", err)
		}
		switch tok := token.(type) {
		case xml.StartElement:
			switch tok.Name.Local {
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			switch tok.Name.Local {
			case "p":
				b.WriteString("\n\n")
			case "tr":
				b.WriteByte('\n')
			}
		case xml.CharData:
			b.Write(tok)
		}
	}
	return b.String(), nil
}

func extractPDFText(ctx context.Context, path string) (string, error) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("PDF extraction requires pdftotext to be installed")
	}
	cmd := exec.CommandContext(ctx, bin, "-layout", "-nopgbrk", "-q", path, "-")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("extract PDF text: %s", msg)
	}
	text := normalizeExtractedText(stdout.String())
	if text == "" {
		return "", fmt.Errorf("PDF %q did not contain extractable text", path)
	}
	return text, nil
}

var referencedPathPattern = regexp.MustCompile(`(?i)(?:^|[\s"'` + "`" + `“”‘’(){}\[\]<>，。！？；：,;])((?:\.{0,2}/|/)?[^\s"'` + "`" + `“”‘’(){}\[\]<>，。！？；：,;]+\.(?:txt|md|docx|pdf))(?:$|[\s"'` + "`" + `“”‘’(){}\[\]<>，。！？；：,;])`)
var referencedDirPattern = regexp.MustCompile(`(?i)(?:^|[\s"'` + "`" + `“”‘’(){}\[\]<>，。！？；：,;])((?:\.{0,2}/|/)?[^\s"'` + "`" + `“”‘’(){}\[\]<>，。！？；：,;]+?)\s*(?:目录(?:里|中)?|文件夹(?:里|中)?|folder|dir)`)

func extractReferencedFiles(input string) []string {
	paths := extractFilesNamedInsideReferencedDirectories(input)
	matches := referencedPathPattern.FindAllStringSubmatch(input, -1)
	seen := make(map[string]bool, len(matches))
	for _, path := range paths {
		seen[path] = true
	}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		candidate := normalizeReferencedPathCandidate(match[1])
		if looksLikeDirectoryPhrase(candidate) && !isExistingFile(candidate) {
			continue
		}
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		paths = append(paths, candidate)
	}
	return paths
}

func extractFilesNamedInsideReferencedDirectories(input string) []string {
	dirMatches := referencedDirPattern.FindAllStringSubmatchIndex(input, -1)
	if len(dirMatches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	paths := make([]string, 0, len(dirMatches))
	for _, match := range dirMatches {
		if len(match) < 4 || match[2] < 0 || match[3] < 0 {
			continue
		}
		dir := normalizeReferencedPathCandidate(input[match[2]:match[3]])
		if dir == "" {
			continue
		}
		segment := input[match[1]:sentenceBoundary(input, match[1])]
		for _, fileMatch := range referencedPathPattern.FindAllStringSubmatch(segment, -1) {
			if len(fileMatch) < 2 {
				continue
			}
			name := normalizeReferencedPathCandidate(fileMatch[1])
			if name == "" {
				continue
			}
			path := name
			if !filepath.IsAbs(name) && filepath.Dir(name) == "." {
				path = filepath.Join(dir, name)
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

func sentenceBoundary(input string, start int) int {
	for idx, r := range input[start:] {
		switch r {
		case '\n', '\r', '，', '。', '！', '？', '；', '：', ',', ';':
			return start + idx
		}
	}
	return len(input)
}

func extractReferencedDirectories(input string) []string {
	matches := referencedDirPattern.FindAllStringSubmatch(input, -1)
	seen := make(map[string]bool, len(matches))
	dirs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		candidate := normalizeReferencedPathCandidate(match[1])
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		dirs = append(dirs, candidate)
	}
	return dirs
}

func normalizeReferencedPathCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	start := -1
	for idx, r := range candidate {
		if r == '.' || r == '/' || r == '_' || r == '-' ||
			(r >= '0' && r <= '9') ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') {
			start = idx
			break
		}
	}
	if start > 0 {
		candidate = candidate[start:]
	}
	return strings.TrimSpace(candidate)
}

func looksLikeDirectoryPhrase(candidate string) bool {
	return strings.Contains(candidate, "目录") || strings.Contains(candidate, "文件夹")
}

func discoverFilesInReferencedDirectories(input string) []string {
	dirs := extractReferencedDirectories(input)
	if len(dirs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	discovered := make([]string, 0, len(dirs))
	hintType := detectWorkspaceTypeHint(input)
	wantsDocx := strings.Contains(strings.ToLower(input), "docx")
	wantsPDF := strings.Contains(strings.ToLower(input), "pdf")
	for _, dir := range dirs {
		candidates, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		bestPath := ""
		bestScore := 0
		for _, entry := range candidates {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if !isSupportedIngestExt(ext) {
				continue
			}
			score := scoreReferencedDirectoryCandidate(hintType, name, ext, wantsDocx, wantsPDF)
			if score > bestScore {
				bestScore = score
				bestPath = filepath.Join(dir, name)
			}
		}
		if bestScore == 0 || bestPath == "" || seen[bestPath] {
			continue
		}
		seen[bestPath] = true
		discovered = append(discovered, bestPath)
	}
	return discovered
}

func isSupportedIngestExt(ext string) bool {
	switch ext {
	case ".txt", ".md", ".docx", ".pdf":
		return true
	default:
		return false
	}
}

func scoreReferencedDirectoryCandidate(hintType string, name string, ext string, wantsDocx bool, wantsPDF bool) int {
	lowerName := strings.ToLower(name)
	score := 0
	switch ext {
	case ".docx":
		score += 10
	case ".pdf":
		score += 8
	case ".md", ".txt":
		score += 4
	}
	if wantsDocx && ext == ".docx" {
		score += 20
	}
	if wantsPDF && ext == ".pdf" {
		score += 20
	}
	switch hintType {
	case WorkspaceTypeResume:
		if ext == ".docx" || ext == ".pdf" {
			score += 20
		}
		if strings.Contains(lowerName, "resume") || strings.Contains(lowerName, "cv") || strings.Contains(name, "简历") {
			score += 40
		}
	case WorkspaceTypeJD:
		if strings.Contains(lowerName, "jd") || strings.Contains(lowerName, "job") || strings.Contains(name, "岗位") || strings.Contains(name, "职位") {
			score += 40
		}
	case WorkspaceTypeProject:
		if strings.Contains(lowerName, "project") || strings.Contains(lowerName, "portfolio") || strings.Contains(name, "项目") {
			score += 40
		}
	}
	if score == 0 && (strings.Contains(lowerName, "resume") || strings.Contains(lowerName, "cv") || strings.Contains(name, "简历")) {
		score += 25
	}
	if score == 0 && (strings.Contains(lowerName, "jd") || strings.Contains(lowerName, "job") || strings.Contains(name, "岗位") || strings.Contains(name, "职位")) {
		score += 25
	}
	return score
}
