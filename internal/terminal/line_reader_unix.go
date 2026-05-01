//go:build darwin || linux || freebsd || netbsd || openbsd

package terminal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

type rawLineReader struct {
	input    *os.File
	output   io.Writer
	reader   *bufio.Reader
	original *unix.Termios
}

func newRawLineReader(input io.Reader, output io.Writer) (LineReader, error) {
	file, ok := input.(*os.File)
	if !ok {
		return nil, nil
	}
	fd := int(file.Fd())
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return nil, nil
	}
	original := *termios
	raw := original
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN | unix.ISIG
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &raw); err != nil {
		return nil, err
	}
	return &rawLineReader{
		input:    file,
		output:   output,
		reader:   bufio.NewReader(file),
		original: &original,
	}, nil
}

func (r *rawLineReader) Close() error {
	if r.original == nil {
		return nil
	}
	return unix.IoctlSetTermios(int(r.input.Fd()), unix.TIOCSETA, r.original)
}

func (r *rawLineReader) ReadLine(prompt string) (string, error) {
	buf := make([]rune, 0, 64)
	cursor := 0
	if err := r.render(prompt, buf, cursor); err != nil {
		return "", err
	}
	for {
		b, err := r.reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch b {
		case '\r', '\n':
			if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
				return "", err
			}
			return string(buf), nil
		case 0x03:
			if _, err := fmt.Fprint(r.output, "^C\r\n"); err != nil {
				return "", err
			}
			return "", io.EOF
		case 0x04:
			if len(buf) == 0 {
				if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
					return "", err
				}
				return "", io.EOF
			}
		case 0x7f, 0x08:
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
				if err := r.render(prompt, buf, cursor); err != nil {
					return "", err
				}
			}
		case 0x01:
			cursor = 0
			if err := r.render(prompt, buf, cursor); err != nil {
				return "", err
			}
		case 0x05:
			cursor = len(buf)
			if err := r.render(prompt, buf, cursor); err != nil {
				return "", err
			}
		case 0x1b:
			_, err := r.handleEscape(&cursor, &buf)
			if err != nil {
				return "", err
			}
			if err := r.render(prompt, buf, cursor); err != nil {
				return "", err
			}
		default:
			ru, err := decodeRuneFromBytes(b, func(n int) ([]byte, error) {
				rest := make([]byte, n)
				_, err := io.ReadFull(r.reader, rest)
				return rest, err
			})
			if err != nil {
				return "", err
			}
			if ru == 0 || ru == utf8.RuneError && b >= utf8.RuneSelf {
				continue
			}
			buf = append(buf[:cursor], append([]rune{ru}, buf[cursor:]...)...)
			cursor++
			if err := r.render(prompt, buf, cursor); err != nil {
				return "", err
			}
		}
	}
}

func (r *rawLineReader) handleEscape(cursor *int, buf *[]rune) (bool, error) {
	next, err := r.reader.ReadByte()
	if err != nil {
		return false, err
	}
	if next != '[' {
		return false, nil
	}
	cmd, err := r.reader.ReadByte()
	if err != nil {
		return false, err
	}
	length := len(*buf)
	switch cmd {
	case 'C':
		if *cursor < length {
			*cursor = *cursor + 1
		}
	case 'D':
		if *cursor > 0 {
			*cursor = *cursor - 1
		}
	case 'H':
		*cursor = 0
	case 'F':
		*cursor = length
	case '3':
		tilde, err := r.reader.ReadByte()
		if err != nil {
			return false, err
		}
		if tilde == '~' && *cursor < length {
			*buf = append((*buf)[:*cursor], (*buf)[*cursor+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (r *rawLineReader) render(prompt string, buf []rune, cursor int) error {
	if _, err := fmt.Fprint(r.output, "\r\x1b[2K"); err != nil {
		return err
	}
	text := string(buf)
	if _, err := fmt.Fprint(r.output, prompt, text); err != nil {
		return err
	}
	tail := string(buf[cursor:])
	if width := displayWidth(tail); width > 0 {
		if _, err := fmt.Fprintf(r.output, "\x1b[%dD", width); err != nil {
			return err
		}
	}
	return nil
}
