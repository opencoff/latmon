// main.go - main for ping latency monitor

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/opencoff/pflag"
)

func main() {
	var ver, help bool

	fs := pflag.NewFlagSet(Z, pflag.ExitOnError)
	fs.SetInterspersed(false)
	fs.BoolVarP(&help, "help", "h", false, "Show this help message and exit")
	fs.BoolVarP(&ver, "version", "", false, "Show program version and exit")

	if err := fs.Parse(os.Args[1:]); err != nil {
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
	if len(args) < 1 {
		Die("insufficient args. Try '%s --help'", Z)
	}

	cmd, err := findCmd(args[0])
	if err != nil {
		Die("%s", err)
	}

	if err = cmd.Run(args[1:]); err != nil {
		Die("%s", err)
	}
}

func usage(fs *pflag.FlagSet, errstr string) {
	var rc int

	if len(errstr) > 0 {
		Warn(errstr)
		rc = 1
	}

	var w strings.Builder

	fmt.Fprintf(&w, `%s: ping latency plotter

Usage: %s [global-options] Command [options] arg [args..]

Available commands:
`, Z, Z)

	cv := listCmds()
	for _, c := range cv {
		fmt.Fprintf(&w, "   %-10s - %s\n", c.Name(), c.Desc())
	}

	w.WriteString("\nGlobal Options:\n")
	os.Stdout.WriteString(w.String())
	fs.PrintDefaults()
	os.Exit(rc)
}

// will be filled by the build script
var ProductVersion = "UNKNOWN"
var RepoVersion = "UNKNOWN"
