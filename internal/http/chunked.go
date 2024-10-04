// streaming chunked-encoding reader

package http

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type ChunkedStreamReader struct {
	rd *connCloser

	// current chunk size that is remaining
	chunksz int
}

func NewChunkedStreamReader(rd *connCloser) *ChunkedStreamReader {
	c := &ChunkedStreamReader{
		rd: rd,
	}
	return c
}

func (c *ChunkedStreamReader) Read(p []byte) (int, error) {
	want := len(p)
	b := p
	tot := 0

	for want > 0 {
		if c.chunksz == 0 {
			n, err := c.readChunkSize()
			if err != nil {
				return tot, fmt.Errorf("chunked-reader: chunk size: %w", err)
			}
			if n == 0 {
				return tot, io.EOF
			}
			c.chunksz = n
		}

		n := min(want, c.chunksz)
		_, err := io.ReadFull(c.rd, b[:n])
		if err != nil {
			return tot, fmt.Errorf("chunked-reader: chunk data: %w", err)
		}

		want -= n
		tot += n
		c.chunksz -= n
		b = b[n:]

		if c.chunksz == 0 {
			if err = c.skipCRLF(); err != nil {
				return tot, fmt.Errorf("chunked-reader: crlf: %w", err)
			}
		}
	}
	return len(p), nil
}

func (c *ChunkedStreamReader) Close() error {
	return c.rd.Close()
}

// readChunkSize reads the chunk size from the stream (hexadecimal format)
func (c *ChunkedStreamReader) readChunkSize() (int, error) {
	line, err := c.rd.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return 0, nil
	}

	size, err := strconv.ParseUint(line, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chunk size: %v", err)
	}
	return int(size), nil
}

// skipCRLF skips the CRLF after a chunk
func (c *ChunkedStreamReader) skipCRLF() error {
	var buf [2]byte

	n, err := io.ReadFull(c.rd, buf[:])
	if err != nil {
		return err
	}

	if n == 2 && buf[0] == '\r' && buf[1] == '\n' {
		return nil
	}

	return errBadTrailer
}

var errBadTrailer = errors.New("invalid CRLF after chunk")
