package runlog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"happyagent/internal/protocol"
)

type Session struct {
	path string
	file *os.File
}

var (
	mu     sync.Mutex
	writer io.Writer = io.Discard

	jsonSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)("api_key"\s*:\s*")([^"]*)(")`),
		regexp.MustCompile(`(?i)("authorization"\s*:\s*")([^"]*)(")`),
		regexp.MustCompile(`(?i)("token"\s*:\s*")([^"]*)(")`),
		regexp.MustCompile(`(?i)("secret"\s*:\s*")([^"]*)(")`),
	}
	textSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)(\S+)`),
		regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)([^\r\n]+)`),
		regexp.MustCompile(`(?i)(bearer\s+)(\S+)`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9._\-]+\b`),
	}
)

func NewSession(root string) (*Session, error) {
	runDir := filepath.Join(root, "logs", time.Now().Format("20060102-150405.000"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("create run log dir %q: %w", runDir, err)
	}

	path := filepath.Join(runDir, "log.md")
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create run log file %q: %w", path, err)
	}

	session := &Session{
		path: path,
		file: file,
	}
	session.write("# Run Log\n\n")
	session.write("Started at: `" + time.Now().Format(time.RFC3339) + "`\n\n")
	return session, nil
}

func (s *Session) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Session) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	return s.file.Close()
}

func (s *Session) Enable() {
	if s == nil || s.file == nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	writer = s.file
}

func Disable() {
	mu.Lock()
	defer mu.Unlock()
	writer = io.Discard
}

func Section(title string, body string) {
	write("## " + title + "\n\n" + strings.TrimSpace(body) + "\n\n")
}

func JSON(title string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		Section(title, "marshal error: "+err.Error())
		return
	}
	CodeBlock(title, "json", string(data))
}

func CodeBlock(title string, language string, body string) {
	body = strings.TrimSpace(body)
	if language == "" {
		language = "text"
	}
	write("## " + title + "\n\n```" + language + "\n" + body + "\n```\n\n")
}

func Linef(format string, args ...any) {
	write(fmt.Sprintf(format, args...) + "\n")
}

func Step(index int, actions []protocol.Action, observation string) {
	var builder strings.Builder
	builder.WriteString("### Actions\n\n")
	builder.WriteString("```json\n")
	data, err := json.MarshalIndent(actions, "", "  ")
	if err != nil {
		builder.WriteString(fmt.Sprintf("{\"marshal_error\":%q}\n", err.Error()))
	} else {
		builder.Write(data)
		builder.WriteString("\n")
	}
	builder.WriteString("```\n\n")
	builder.WriteString("### Observation\n\n")
	builder.WriteString("```text\n")
	builder.WriteString(strings.TrimSpace(observation))
	builder.WriteString("\n```\n")
	write(fmt.Sprintf("## Step %d\n\n%s\n", index, builder.String()))
}

func write(content string) {
	content = sanitize(content)
	mu.Lock()
	defer mu.Unlock()
	if writer == nil {
		return
	}
	_, _ = io.WriteString(writer, content)
}

func (s *Session) write(content string) {
	if s == nil || s.file == nil {
		return
	}
	content = sanitize(content)
	_, _ = io.WriteString(s.file, content)
}

func sanitize(content string) string {
	for _, pattern := range jsonSecretPatterns {
		content = pattern.ReplaceAllString(content, `${1}[REDACTED]${3}`)
	}
	for _, pattern := range textSecretPatterns {
		switch pattern.String() {
		case `\bsk-[A-Za-z0-9._\-]+\b`:
			content = pattern.ReplaceAllString(content, "[REDACTED]")
		default:
			content = pattern.ReplaceAllString(content, `${1}[REDACTED]`)
		}
	}
	return content
}
