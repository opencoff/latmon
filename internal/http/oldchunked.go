// old chunked reader impl

package http

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ChunkedReader struct to handle chunked transfer encoding
type ChunkedReader struct {
	reader *bufio.Reader
	raw    io.ReadCloser
}

// NewChunkedReader creates a new ChunkedReader from a connection or any io.Reader
func NewChunkedReader(rd *bufio.Reader, raw io.ReadCloser) *ChunkedReader {
	return &ChunkedReader{
		reader: rd,
		raw:    raw,
	}
}

func (c *ChunkedReader) Close() error {
	return c.raw.Close()
}

// Read implements the io.Reader interface for ChunkedReader
// It reads and decodes the chunked transfer encoding from the stream
func (cr *ChunkedReader) Read(p []byte) (int, error) {
	var tot int

	var i int
	want := len(p)
	b := p

	for want > 0 {
		n, err := cr.readChunkSize()
		if err != nil {
			return tot, err
		}
		if n == 0 {
			return tot, io.EOF
		}

		if n > want {
			// this is an error.
			// We don't have a buffer large enough
			// but we've already read the chunk size!
			return tot, fmt.Errorf("chunked-reader: buffer too small (%d bytes of %d left); want %d",
				want, len(p), n)
		}

		fmt.Printf("Chunk #%d: %d bytes\n", i, n)

		// Read the chunk data
		_, err = io.ReadFull(cr.reader, b[:n])
		if err != nil {
			return tot, fmt.Errorf("chunked-reader: chunk data: %w", err)
		}

		fmt.Printf("|")
		os.Stdout.Write(b)
		fmt.Printf("|\n")

		// now skip the crlf
		if err = cr.skipCRLF(); err != nil {
			return tot, fmt.Errorf("chunked-reader: crlf: %w", err)
		}
		want -= n
		b = b[n:]
		i += 1
	}
	return len(p), nil
}

// readChunkSize reads the chunk size from the stream (hexadecimal format)
func (cr *ChunkedReader) readChunkSize() (int, error) {
	line, err := cr.reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	size, err := strconv.ParseInt(line, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chunk size: %v", err)
	}
	return int(size), nil
}

// skipCRLF skips the CRLF after a chunk
func (cr *ChunkedReader) skipCRLF() error {
	var buf [2]byte

	n, err := io.ReadFull(cr.reader, buf[:])
	if err != nil {
		return err
	}

	fmt.Printf("CRLF: %d bytes => |%x|\n", n, buf[:n])

	crlf := string(buf[:n])
	if crlf != "\r\n" {
		return errors.New("invalid CRLF after chunk")
	}
	return nil
}
