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
	"strings"
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
