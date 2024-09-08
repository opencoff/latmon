package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/opencoff/pflag"
)

func main() {
	var interval, timeout time.Duration
	var help, ver bool
	var dir string
	var bsz int

	fs := pflag.NewFlagSet(Z, pflag.ExitOnError)
	fs.DurationVarP(&interval, "every", "i", 2*time.Second, "Send pings every `I` interval apart")
	fs.IntVarP(&bsz, "batch-size", "b", 3600, "Collect 'B' samples per measurement run")
	fs.DurationVarP(&timeout, "timeout", "t", 2*time.Second, "Receive deadline wait period")
	fs.BoolVarP(&help, "help", "h", false, "show this help message and exit")
	fs.BoolVarP(&ver, "version", "", false, "show program version and exit")
	fs.StringVarP(&dir, "output-dir", "d", ".", "Put charts in directory `D`")

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

	m := NewMeasurer(WithOutputDir(dir), WithBatchSize(bsz))

	ctx := context.Background()
	for _, a := range args {
		proto, host, port, err := parsePinger(a)
		if err != nil {
			Die(err.Error())
		}

		opt := PingOpts{
			Host:     host,
			Port:     port,
			Proto:    proto,
			Interval: interval,
			Timeout:  timeout,
		}

		fmt.Printf("proto %s, host %s, port %d\n", proto, host, port)
		switch proto {
		case "icmp":
			p, ich, err := NewIcmp(ctx, opt)
			if err != nil {
				Die("%s", err)
			}
			m.AddIcmp(host, p, ich)

		case "https":
			h, hch, err := NewHttps(ctx, opt)
			if err != nil {
				Die("%s", err)
			}
			m.AddHttps(fmt.Sprintf("%s:%d", host, port), h, hch)
		default:
			Warn("proto %s: TBD", proto)
		}
	}

	// now the work has kicked off. Wait for a signal to terminate
	sigchan := make(chan os.Signal, 4)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	signal.Ignore(syscall.SIGPIPE, syscall.SIGFPE)

	// Now wait for signals to arrive
	for {
		_ = <-sigchan
		//t := s.(syscall.Signal)

		//log.Info("Caught signal %d; Terminating ..\n", int(t))
		break
	}

	m.Stop()
}

func parsePinger(s string) (proto, host string, port uint16, err error) {
	v := strings.Split(s, ":")
	if len(v) < 2 {
		err = fmt.Errorf("malformed ping specification '%s'", s)
		return
	}

	proto = strings.ToLower(v[0])
	host = v[1]

	want := 2
	switch proto {
	case "icmp":
		// nothing to do

	case "https", "quic":
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
