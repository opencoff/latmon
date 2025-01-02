// main.go - main for ping latency monitor

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/opencoff/latmon/internal/plot"
	"github.com/opencoff/pflag"
)

const (
	_PlotCmd  = "plot"
	_PlotDesc = "plot a chart from one or more CSVs"
)

type plotCmd struct{}

func (p *plotCmd) Name() string {
	return _PlotCmd
}

func (p *plotCmd) Desc() string {
	return _PlotDesc
}

func (p *plotCmd) Run(args []string) error {
	var help, force bool
	var outfile string

	fs := pflag.NewFlagSet(_PlotCmd, pflag.ExitOnError)
	fs.BoolVarP(&help, "help", "h", false, "Show this help message and exit")
	fs.BoolVarP(&force, "force", "", false, "Force overwrite of output file")
	fs.StringVarP(&outfile, "output", "o", "-", "Plot the chart in file `F`")

	err := fs.Parse(args)
	if err != nil {
		return err
	}

	if help {
		x := fmt.Sprintf(`%s: %s

Usage: %s [options] CSV [CSV...]

Where CSV is a previously collected CSV file.

Options:
`, _PlotCmd, _PlotDesc, _PlotCmd)

		os.Stdout.Write([]byte(x))
		fs.PrintDefaults()
		return nil
	}

	args = fs.Args()
	if len(args) < 0 {
		return fmt.Errorf("plot: insufficient args")
	}

	var wr io.Writer = os.Stdout
	if len(outfile) > 0 && outfile != "-" {
		fl := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		if !force {
			fl |= os.O_EXCL
		}

		fd, err := os.OpenFile(outfile, fl, 0600)
		if err != nil {
			return fmt.Errorf("can't create %s: %w", outfile, err)
		}
		defer fd.Close()
		wr = fd
	}

	pc, err := p.readFiles(args)
	if err != nil {
		return err
	}

	return plot.Chart(pc, wr)
}

type filearg struct {
	nm string
	st fs.FileInfo
}

func (p *plotCmd) readFiles(args []string) (*plot.Columns, error) {
	// gather and sort the filenames
	fa := make([]filearg, 0, len(args))
	for _, fn := range args {
		st, err := os.Lstat(fn)
		if err != nil {
			return nil, fmt.Errorf("plot: can't stat %s: %w", fn, err)
		}
		fa = append(fa, filearg{fn, st})
	}

	sort.Sort(byMtime(fa))

	// now, we can read each of the csv and merge them
	var cref [][]time.Duration
	var names []string

	for _, x := range fa {
		rd, err := os.Open(x.nm)
		if err != nil {
			return nil, fmt.Errorf("can't open %s: %w", x.nm, err)
		}

		// XXX We assume every CSV has same set of cols
		hdr, vals, err := p.readCSV(rd)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", x.nm, err)
		}

		//fmt.Printf("%s: %d records (%d cols)\n", x.nm, len(vals[0]), len(hdr))

		if names == nil {
			names = hdr
			cref = make([][]time.Duration, len(names))
		}
		for i := range vals {
			cref[i] = append(cref[i], vals[i]...)
		}
	}

	minlen := 1000000000
	for i := range cref {
		minlen = min(minlen, len(cref[i]))
	}
	pc := &plot.Columns{
		Name:   fmt.Sprintf("merged plot"),
		Start:  fa[0].st.ModTime(),
		Names:  names,
		Colref: cref,
		Minlen: minlen,
	}
	return pc, nil
}

func (p *plotCmd) readCSV(rd io.ReadCloser) ([]string, [][]time.Duration, error) {
	r := csv.NewReader(rd)
	defer rd.Close()

	hdr, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	vals := make([][]time.Duration, len(hdr))
	for {
		v, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		for i, s := range v {
			d, err := strconv.ParseInt(s, 0, 64)
			if err != nil {
				return nil, nil, fmt.Errorf("can't parse %s: %w", s, err)
			}
			vals[i] = append(vals[i], time.Duration(d))
		}
	}

	return hdr, vals, nil
}

type byMtime []filearg

func (v byMtime) Len() int {
	return len(v)
}

func (v byMtime) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v byMtime) Less(i, j int) bool {
	a := v[i]
	b := v[j]

	return a.st.ModTime().Before(b.st.ModTime())
}

var _ Cmd = &plotCmd{}

func init() {
	registerCmd(&plotCmd{})
}
