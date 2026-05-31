package career

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"happyagent/internal/tools"
)

type ExtractedDocument struct {
	Text          string
	Extractor     string
	MIMEType      string
	ExtractStatus string
	ExtractError  string
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
	text, err := tools.ExtractDOCXText(path)
	if err != nil {
		return "", err
	}
	return normalizeExtractedText(text), nil
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
