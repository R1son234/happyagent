package career

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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
		if candidate == "" || seen[candidate] || hasEquivalentReferencedPath(seen, candidate) {
			continue
		}
		seen[candidate] = true
		paths = append(paths, candidate)
	}
	return paths
}

func hasEquivalentReferencedPath(seen map[string]bool, candidate string) bool {
	if filepath.IsAbs(candidate) || filepath.Dir(candidate) != "." {
		return false
	}
	for path := range seen {
		if filepath.Base(path) == candidate {
			return true
		}
	}
	return false
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

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isSupportedIngestExt(ext string) bool {
	switch ext {
	case ".txt", ".md", ".docx", ".pdf":
		return true
	default:
		return false
	}
}
