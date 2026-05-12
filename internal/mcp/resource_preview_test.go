package mcp

import (
	"testing"
	"unicode/utf8"
)

func TestPreviewResourceTextPreservesUTF8(t *testing.T) {
	// "你好世界" = 4 Chinese characters = 12 bytes (3 bytes each)
	content := "你好世界hello"
	// Slice at byte 10 which is in the middle of a 3-byte char
	result := previewResourceText(content, 0, 10, 10)
	if !utf8.ValidString(result) {
		t.Fatalf("result is not valid UTF-8: %q", result)
	}
}

func TestSafeByteSlicePreservesUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
	}{
		{"ascii", "hello world", 5},
		{"chinese", "你好世界hello", 10},
		{"emoji", "hi👋🌍end", 6},
		{"within limit", "short", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeByteSlice(tt.input, tt.maxBytes)
			if len(result) > tt.maxBytes {
				t.Fatalf("result exceeds maxBytes: len=%d max=%d", len(result), tt.maxBytes)
			}
			if !utf8.ValidString(result) {
				t.Fatalf("result is not valid UTF-8: %q", result)
			}
		})
	}
}

func TestTruncateResourceTextPreservesUTF8(t *testing.T) {
	content := "这是一段中文内容，用于测试截断是否保持UTF-8有效性。"
	result := truncateResourceText(content, 20)
	if !utf8.ValidString(result) {
		t.Fatalf("truncated result is not valid UTF-8: %q", result)
	}
}
