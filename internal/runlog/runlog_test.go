package runlog

import (
	"strings"
	"testing"
)

func TestSanitizeRedactsJSONSecrets(t *testing.T) {
	input := `{"api_key":"secret-key","authorization":"Bearer abc123","token":"xyz","secret":"hidden"}`

	output := sanitize(input)

	for _, raw := range []string{"secret-key", "abc123", "xyz", "hidden"} {
		if strings.Contains(output, raw) {
			t.Fatalf("sanitize() leaked secret %q in %q", raw, output)
		}
	}
	if !strings.Contains(output, `"api_key":"[REDACTED]"`) {
		t.Fatalf("sanitize() did not redact api_key: %q", output)
	}
}

func TestSanitizeRedactsTextSecrets(t *testing.T) {
	input := "api_key=secret123\nauthorization: Bearer token456\nraw sk-abcDEF123"

	output := sanitize(input)

	for _, raw := range []string{"secret123", "token456", "sk-abcDEF123"} {
		if strings.Contains(output, raw) {
			t.Fatalf("sanitize() leaked secret %q in %q", raw, output)
		}
	}
	if !strings.Contains(output, "api_key=[REDACTED]") {
		t.Fatalf("sanitize() did not redact api_key assignment: %q", output)
	}
	if !strings.Contains(output, "authorization: [REDACTED]") {
		t.Fatalf("sanitize() did not redact authorization header: %q", output)
	}
}
