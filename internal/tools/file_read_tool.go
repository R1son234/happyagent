package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	defaultFileReadMaxBytes = 16 * 1024
	minFileReadMaxBytes     = 256
)

type FileReadTool struct {
	resolver *RootedPathResolver
}

func NewFileReadTool(root string) (*FileReadTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileReadTool{resolver: resolver}, nil
}

func (t *FileReadTool) Definition() Definition {
	return Definition{
		Name:        "file_read",
		Description: "Read a text file under the configured root directory. Supports optional line ranges; large files are truncated automatically and binary files return a summary instead of raw bytes.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"max_bytes":{"type":"integer","minimum":256,"description":"Optional maximum number of bytes to return. Defaults to 16384."},"start_line":{"type":"integer","minimum":1,"description":"Optional inclusive start line for partial reads."},"end_line":{"type":"integer","minimum":1,"description":"Optional inclusive end line for partial reads."}},"required":["path"]}`,
	}
}

func (t *FileReadTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path      string `json:"path"`
		MaxBytes  int    `json:"max_bytes"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_read arguments: %w", err)
	}
	if err := validateFileReadRange(input.StartLine, input.EndLine); err != nil {
		return Result{}, err
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}

	output, err := readFilePreview(path, normalizeFileReadLimit(input.MaxBytes), input.StartLine, input.EndLine)
	if err != nil {
		return Result{}, fmt.Errorf("read file %q: %w", path, err)
	}

	return Result{Output: output}, nil
}

func normalizeFileReadLimit(limit int) int {
	if limit <= 0 {
		return defaultFileReadMaxBytes
	}
	if limit < minFileReadMaxBytes {
		return minFileReadMaxBytes
	}
	return limit
}

func validateFileReadRange(startLine int, endLine int) error {
	if startLine < 0 || endLine < 0 {
		return fmt.Errorf("line range must use positive line numbers")
	}
	if endLine > 0 && startLine == 0 {
		return fmt.Errorf("start_line is required when end_line is set")
	}
	if startLine > 0 && endLine > 0 && endLine < startLine {
		return fmt.Errorf("end_line must be greater than or equal to start_line")
	}
	return nil
}

func readFilePreview(path string, maxBytes int, startLine int, endLine int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", path)
	}
	if startLine > 0 {
		return readFileLineRange(file, path, maxBytes, startLine, endLine)
	}

	size := info.Size()
	if size <= int64(maxBytes) {
		data, err := io.ReadAll(file)
		if err != nil {
			return "", err
		}
		return renderWholeFilePreview(data, size), nil
	}

	headBytes := maxBytes / 2
	tailBytes := maxBytes - headBytes
	head := make([]byte, headBytes)
	if _, err := io.ReadFull(file, head); err != nil {
		return "", err
	}

	tail := make([]byte, tailBytes)
	if _, err := file.ReadAt(tail, size-int64(tailBytes)); err != nil {
		return "", err
	}

	data := append(head, tail...)
	if looksBinary(data) {
		return fmt.Sprintf("[binary file omitted: size=%d bytes, preview_limit=%d bytes]", size, headBytes+tailBytes), nil
	}
	return renderTruncatedPreview(data[:headBytes], data[headBytes:], int(size)-maxBytes, fmt.Sprintf("%d-byte file", size)), nil
}

func readFileLineRange(file *os.File, path string, maxBytes int, startLine int, endLine int) (string, error) {
	reader := bufio.NewReader(file)
	lineNumber := 1
	var selected strings.Builder
	selectedAny := false

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if lineNumber >= startLine && (endLine == 0 || lineNumber <= endLine) {
				selected.WriteString(line)
				selectedAny = true
			}
			if endLine > 0 && lineNumber >= endLine {
				break
			}
			lineNumber++
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	if !selectedAny {
		if endLine > 0 {
			return "", fmt.Errorf("line range %d-%d is outside file %q", startLine, endLine, path)
		}
		return "", fmt.Errorf("line range starting at %d is outside file %q", startLine, path)
	}

	data := []byte(selected.String())
	if looksBinary(data) {
		return "[binary file omitted for requested line range]", nil
	}
	return renderPartialPreview(data, maxBytes, "requested line range"), nil
}

func renderWholeFilePreview(data []byte, size int64) string {
	if looksBinary(data) {
		return fmt.Sprintf("[binary file omitted: size=%d bytes]", size)
	}
	return string(data)
}

func renderPartialPreview(data []byte, maxBytes int, scope string) string {
	if len(data) <= maxBytes {
		return string(data)
	}

	headBytes := maxBytes / 2
	tailBytes := maxBytes - headBytes
	return renderTruncatedPreview(data[:headBytes], data[len(data)-tailBytes:], len(data)-maxBytes, scope)
}

func renderTruncatedPreview(head []byte, tail []byte, omittedBytes int, scope string) string {
	headText := string(head)
	tailText := string(tail)

	var builder strings.Builder
	builder.WriteString(headText)
	if !strings.HasSuffix(headText, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("[file_read truncated %d bytes]", omittedBytes))
	builder.WriteString("\n\n")
	builder.WriteString(tailText)
	if !strings.HasSuffix(tailText, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("\n[file_read showing first %d bytes and last %d bytes of %s]", len(head), len(tail), scope))
	return builder.String()
}

func looksBinary(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	return !utf8.Valid(data)
}
