package terminal

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

type LineReader interface {
	ReadLine(prompt string) (string, error)
	Close() error
}

type fallbackLineReader struct {
	reader *bufio.Reader
	output io.Writer
}

func NewLineReader(input io.Reader, output io.Writer) (LineReader, error) {
	if raw, err := newRawLineReader(input, output); err != nil {
		return nil, err
	} else if raw != nil {
		return raw, nil
	}
	return &fallbackLineReader{
		reader: bufio.NewReader(input),
		output: output,
	}, nil
}

func (r *fallbackLineReader) ReadLine(prompt string) (string, error) {
	if _, err := fmt.Fprint(r.output, prompt); err != nil {
		return "", err
	}
	line, err := r.reader.ReadString('\n')
	if err != nil {
		if err == io.EOF && line != "" {
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (r *fallbackLineReader) Close() error {
	return nil
}

func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		width += runeDisplayWidth(r)
	}
	return width
}

func runeDisplayWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), unicode.Is(unicode.Cf, r):
		return 0
	case isWideRune(r):
		return 2
	default:
		return 1
	}
}

func isWideRune(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff) ||
		(r >= 0x20000 && r <= 0x3fffd))
}

func decodeRuneFromBytes(first byte, readMore func(int) ([]byte, error)) (rune, error) {
	if first < utf8.RuneSelf {
		return rune(first), nil
	}
	size := 0
	switch {
	case first&0xe0 == 0xc0:
		size = 2
	case first&0xf0 == 0xe0:
		size = 3
	case first&0xf8 == 0xf0:
		size = 4
	default:
		return utf8.RuneError, nil
	}
	buf := []byte{first}
	if size > 1 {
		rest, err := readMore(size - 1)
		if err != nil {
			return 0, err
		}
		buf = append(buf, rest...)
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}
