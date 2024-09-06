package main

import (
	"os"
	"strconv"
	"strings"
	"time"
	"fmt"

	"github.com/opencoff/pflag"
)


func main() {
	var interval, timeout time.Duration
	var help, ver bool

	fs := pflag.NewFlagSet(Z,   pflag.ExitOnError)
	fs.DurationVarP(&interval,  "every", "i", 2 * time.Second, "Send pings every `I` interval apart")

	fs.DurationVarP(&timeout, "timeout", "t", 2 * time.Second, "Receive deadline wait period")
	fs.BoolVarP(&help, "help", "h", false, "show this help message and exit")
	fs.BoolVarP(&ver, "version", "", false, "show program version and exit")

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

	for _, a := range args {
		proto, host, port, err := parsePinger(a)
		if err != nil {
			Die(err.Error())
		}

		fmt.Printf("proto %s, host %s, port %d\n", proto, host, port)
	}
}

func parsePinger(s string) (proto, host string, port uint16, err error) {
	v := strings.Split(s, ":")
	if len(v) < 2 {
		err = fmt.Errorf("malformed ping specification '%s'", s)
		return
	}

	proto = v[0]
	host = v[1]

	want := 2
	switch proto {
	case "icmp", "ICMP":
		// nothing to do
	case "https", "quic", "QUIC":
		if len(v) != want {
			err = fmt.Errorf("missing port# for proto '%s'", proto)
			return
		}
	default:
		err = fmt.Errorf("unknown proto '%s'", proto)
		return
	}

	if len(v) > 2 {
		var pv uint64
		pv, err = strconv.ParseUint(v[2], 0, 16)
		if err != nil {
			return
		}
		port = uint16(pv & 0xffff)
	}
	return
}


func usage(fs *pflag.FlagSet, errstr string) {
	var rc int

	if len(errstr) > 0 {
		Warn(errstr)
		rc = 1
	}

	x := fmt.Sprintf(`%s: ping latency plotter

Usage: %s [options] PINGER [PINGER..]

Where PINGER is of the form:

	icmp:HOST
	https:HOST:port
	quic:HOST:port

Options:
`, Z, Z)
	os.Stdout.Write([]byte(x))
	fs.PrintDefaults()
	os.Exit(rc)
}

// will be filled by the build script
var ProductVersion = "UNKNOWN"
var RepoVersion = "UNKNOWN"
