package mcp

import "strings"

const namePrefix = "mcp"

func CanonicalToolName(serverName string, remoteName string) string {
	return namePrefix + "__" + normalizeName(serverName) + "__" + normalizeName(remoteName)
}

func CanonicalPromptName(serverName string, promptName string) string {
	return namePrefix + "__" + normalizeName(serverName) + "__" + normalizeName(promptName)
}

func normalizeName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
