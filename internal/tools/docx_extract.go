package tools

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ExtractDOCXText extracts plain text from a .docx file by parsing its
// word/document.xml. It returns the extracted text content.
func ExtractDOCXText(path string) (string, error) {
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
