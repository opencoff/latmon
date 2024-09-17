// main.go - main for ping latency monitor merge tool

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"io/fs"
	"sync"
	"sort"

	"github.com/opencoff/go-utils"
	"github.com/opencoff/pflag"
)

func main() {
	var help, ver bool
	var outfile string

	fs := pflag.NewFlagSet(Z, pflag.ExitOnError)
	fs.StringVarP(&outfile, "output", "o", "-", "Write output to file `F`")
	fs.BoolVarP(&help, "help", "h", false, "Show this help message and exit")
	fs.BoolVarP(&ver, "version", "", false, "Show program version and exit")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		Die("%s", err)
	}

	if help {
		usage(fs, "")
	}

	if ver {
		fmt.Printf("%s: %s [%s]\n", Z, ProductVersion, RepoVersion)
		os.Exit(0)
	}

	args := fs.Args()
	if len(args) < 0 {
		usage(fs, "insufficient args")
	}

	var fd io.WriteCloser = os.Stdout

	if len(outfile) > 0 && outfile != "-" {
		sf, err := utils.NewSafeFile(outfile, 0, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			Die("%s", err)
		}

		defer sf.Abort()

		fd = sf
	}


	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	wg.Add(1)
	go merge(fd, args, &wg, ctx)

	go func() {
		// now the work has kicked off. Wait for a signal to terminate
		sigchan := make(chan os.Signal, 4)
		signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		signal.Ignore(syscall.SIGPIPE, syscall.SIGFPE)

		done := ctx.Done()

		// Now wait for signals to arrive
		for {
			select {
			case <-done:
				return

			case <-sigchan:
				cancel()
				return
			}
		}
	}()

	wg.Wait()
	cancel()
}

type byMtime []file

func (r byMtime) Len() int {
	return len(r)
}

func (r byMtime) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r byMtime) Less(i, j int) bool {
	a := &r[i]
	b := &r[j]

	lhs := a.st.ModTime()
	rhs := b.st.ModTime()
	return lhs.Compare(rhs) < 0
}

// We need to sort the files by mtime with the oldest file first.
// we will use an aux data structure to make it easy
type file struct {
	fd *os.File
	st  fs.FileInfo
}

// goroutine to do the work while the main thread waits for signals
func merge(fd io.WriteCloser, args []string, wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()

	filev := make([]file, 0, len(args))
	for _, nm := range args {
		fr, err := os.Open(nm)
		if err != nil {
			Die("%s", err)
		}

		st, err := fr.Stat()
		if err != nil {
			fr.Close()
			Die("%s: %s", nm, err)
		}

		fe := file{fr, st}
		filev = append(filev, fe)
	}

	// now we sort by timestamp
	sort.Sort(byMtime(filev))

	// Now merge 'em
	m := newMerger(fd)
	done := ctx.Done()
	for i := range filev {
		fe := &filev[i]
		nm := fe.fd.Name()

		fmt.Printf("+ %s ..\n", nm)
		err := m.addFd(fe.fd)

		fe.fd.Close()
		if err != nil {
			Die("%s: %s", nm, err)
		}

		select {
		case <-done:
			return
		default:
		}
	}

	err := m.Close()
	if err != nil {
		Warn("%s", err)
	}
}

func usage(fs *pflag.FlagSet, errstr string) {
	var rc int

	if len(errstr) > 0 {
		Warn(errstr)
		rc = 1
	}

	x := fmt.Sprintf(`%s: Merge one or more latmon csv data

Usage: %s [options] file.csv [file.csv...]

Options:
`, Z, Z)
	os.Stdout.Write([]byte(x))
	fs.PrintDefaults()
	os.Exit(rc)
}

// will be filled by the build script
var ProductVersion = "UNKNOWN"
var RepoVersion = "UNKNOWN"
