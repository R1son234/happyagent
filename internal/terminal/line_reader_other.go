//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package terminal

import "io"

func newRawLineReader(input io.Reader, output io.Writer) (LineReader, error) {
	return nil, nil
}
