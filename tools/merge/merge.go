// merge.go - merge logic

package main


import (
	"fmt"
	"io"
	"os"
	"encoding/csv"
	"errors"
)


type merger struct {
	wr io.WriteCloser

	cwr *csv.Writer

	wroteHeader bool
}


func newMerger(fd io.WriteCloser) *merger {
	m := &merger{
		wr: fd,
		cwr: csv.NewWriter(fd),
	}
	return m
}


type rec struct {
	data []string
	err error
}

func writeAll(fd io.Writer, d []byte) error {
	n, err := fd.Write(d)
	if err != nil {
		return err
	}
	if n != len(d) {
		return fmt.Errorf("partial write: exp %d, wrote %d", len(d), n)
	}
	return nil
}

// read async and merge in a separate writer thread
func (m *merger) addFd(fd *os.File) error {
	nm := fd.Name()
	ch := m.startReader(fd)

	// read the first line
	r, ok := <-ch

	// empty file
	if !ok {
		return nil
	}

	if r.err != nil {
		Die("%s: %s", nm, r.err)
	}

	if !m.wroteHeader {
		if err := m.cwr.Write(r.data); err != nil {
			Die("%s: %s", nm, err)
		}
		m.wroteHeader = true
	}

	for r := range ch {
		if r.err != nil {
			Die("%s: %s", nm, r.err)
		}
		if err := m.cwr.Write(r.data); err != nil {
			Die("%s: %s", nm, err)
		}
	}
	return nil
}

// start an async reader that feeds records into a channel
func (m *merger) startReader(fd io.Reader) chan rec {
	r := csv.NewReader(fd)
	//r.ReuseRecord = true
	r.TrimLeadingSpace = true

	ch := make(chan rec, 1)
	go func(ch chan rec) {
		for {
			v, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				ch <- rec{err: err}
			}
			ch <- rec{data: v}
		}
		close(ch)
	}(ch)
	return ch
}

func (m *merger) Close() error {
	m.cwr.Flush()
	return m.wr.Close()
}
