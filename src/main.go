package main

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func readFile(fd io.Reader) ([]time.Duration, error) {
	d := make([]time.Duration, 0, 128)
	sc := bufio.NewScanner(fd)
	n := 0
	bi := 0
	big := float64(0.0)
	for ; sc.Scan(); n++ {
		s := strings.TrimSpace(sc.Text())
		if len(s) == 0 || s[0] == '#' {
			continue
		}
		// read this as a float
		r, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}

		// implicitly, we know these are in ms
		z := r * float64(time.Millisecond)
		if z > big {
			big = z
			bi = n
		}
		d = append(d, time.Duration(z))
	}

	// now remove the biggest outlier
	before := d[:bi]
	after := d[bi+1:]
	d = append(before, after...)
	/*
		if len(d) < 1800 {
			for len(d) < 1800 {
				d = append(d, d...)
			}
			d = d[:1800]
		}
	*/
	return d, nil
}

func makeRtt(d []time.Duration) []Rtt {
	r := make([]Rtt, len(d))
	for i, x := range d {
		o := &r[i]
		o.Ping = float64(x.Milliseconds())
		o.Tcp = o.Ping * 1.25
		o.Tls = o.Tcp * 1.31
		o.Http = o.Tls * 1.18
	}
	return r
}

func main() {
	var d []time.Duration
	var err error

	if len(os.Args) < 2 {
		d, err = readFile(os.Stdin)
	} else {
		var fd *os.File
		fn := os.Args[1]
		fd, err = os.Open(fn)
		if err == nil {
			d, err = readFile(fd)
			fd.Close()
		}
	}

	if err != nil {
		Die("%s", err)
	}

	// convert this into a []Rtt
	rtt := makeRtt(d)

	// Plot the durations
	if err := plotDurations(rtt, "chart.html"); err != nil {
		Die("%s", err)
	}
}
