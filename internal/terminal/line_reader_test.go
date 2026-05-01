package terminal

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestFallbackLineReaderReadsLine(t *testing.T) {
	var output bytes.Buffer
	reader, err := NewLineReader(strings.NewReader("hello\n"), &output)
	if err != nil {
		t.Fatalf("NewLineReader() error = %v", err)
	}
	defer reader.Close()
	line, err := reader.ReadLine("prompt> ")
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if line != "hello" {
		t.Fatalf("line = %q, want hello", line)
	}
	if output.String() != "prompt> " {
		t.Fatalf("prompt output = %q", output.String())
	}
}

func TestDisplayWidthCountsWideRunes(t *testing.T) {
	if got := displayWidth("abc"); got != 3 {
		t.Fatalf("displayWidth(ascii) = %d, want 3", got)
	}
	if got := displayWidth("我a"); got != 3 {
		t.Fatalf("displayWidth(mixed) = %d, want 3", got)
	}
}

func TestDecodeRuneFromBytes(t *testing.T) {
	rest := []byte{0xbd, 0xa0}
	r, err := decodeRuneFromBytes(0xe4, func(n int) ([]byte, error) {
		if n != len(rest) {
			t.Fatalf("unexpected read size %d", n)
		}
		return rest, nil
	})
	if err != nil {
		t.Fatalf("decodeRuneFromBytes() error = %v", err)
	}
	if r != '你' {
		t.Fatalf("rune = %q, want %q", r, '你')
	}
}

func TestFallbackLineReaderEOFWithPartialLine(t *testing.T) {
	var output bytes.Buffer
	reader, err := NewLineReader(strings.NewReader("hello"), &output)
	if err != nil {
		t.Fatalf("NewLineReader() error = %v", err)
	}
	defer reader.Close()
	line, err := reader.ReadLine("prompt> ")
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if line != "hello" {
		t.Fatalf("line = %q, want hello", line)
	}
	_, err = reader.ReadLine("prompt> ")
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
