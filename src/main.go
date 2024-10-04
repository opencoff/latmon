// main.go - main for ping latency monitor

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

	logger "github.com/opencoff/go-logger"
	"github.com/opencoff/pflag"
)

const (
	_DefaultBatchSize int = 3600
)

func main() {
	var interval, timeout time.Duration
	var help, ver bool
	var dir, logdest, lvl string
	var bsz int

	fs := pflag.NewFlagSet(Z, pflag.ExitOnError)
	fs.DurationVarP(&interval, "every", "i", 2*time.Second, "Send pings every `I` interval apart")
	fs.IntVarP(&bsz, "batch-size", "b", _DefaultBatchSize, "Collect 'B' samples per measurement run")
	fs.DurationVarP(&timeout, "timeout", "t", 2*time.Second, "Set rx deadline to `T` seconds")
	fs.BoolVarP(&help, "help", "h", false, "Show this help message and exit")
	fs.BoolVarP(&ver, "version", "", false, "Show program version and exit")
	fs.StringVarP(&dir, "output-dir", "d", ".", "Put charts in directory `D`")
	fs.StringVarP(&logdest, "log", "L", "SYSLOG", "Send logs to destination `L`")
	fs.StringVarP(&lvl, "log-level", "", "INFO", "Log at priority `P`")

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

	// samples per day can't be smaller than batchsize
	perDay := int((86400 * time.Second) / interval)
	if bsz >= perDay {
		Die("batch-size is greater than total samples per day (%d)", perDay)
	}

	args := fs.Args()
	if len(args) < 0 {
		usage(fs, "insufficient args")
	}

	prio, ok := logger.ToPriority(lvl)
	if !ok {
		Die("Unknown log level '%s'", lvl)
	}

	log, err := logger.NewLogger(logdest, prio, Z, logger.Ldate|logger.Ltime|logger.Lmicroseconds|logger.Lfileloc)
	if err != nil {
		Die("can't create logger: %s", err)
	}

	log.Info("Starting latency monitor [%s, %s]; batchsize=%d interval=%s timeout=%s",
		ProductVersion, RepoVersion, bsz, interval, timeout)

	m := NewMeasurer(WithOutputDir(dir), WithBatchSize(bsz), WithLogger(log))
	ctx := context.Background()
	seen := make(map[string]bool)
	for _, a := range args {
		proto, host, port, err := parsePinger(a)
		if err != nil {
			Die(err.Error())
		}

		k := fmt.Sprintf("%s:%s:%d", proto, host, port)
		if saw := seen[k]; saw {
			Warn("%s: %s:%d - duplicate; skipping ..", proto, host, port)
			continue
		}

		opt := PingOpts{
			Host:     host,
			Port:     port,
			Proto:    proto,
			Interval: interval,
			Timeout:  timeout,
			Logger:   log,
		}

		switch proto {
		case "https":
			h, hch, err := NewHttps(ctx, opt)
			if err != nil {
				Die("%s", err)
			}
			m.AddHttps(host, h, hch)
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
		s := <-sigchan
		t := s.(syscall.Signal)

		log.Info("Caught signal %d; Terminating ..\n", int(t))
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

	// setup defaults for the port
	switch proto {
	case "http":
		port = 80
	case "https":
		port = 443
	//case "quic":

	default:
		err = fmt.Errorf("unknown proto '%s'", proto)
		return
	}

	// and allow user to override it
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

Usage: %s [options] HOST [HOST..]

Where HOST is of the form:

	https:hostname[:port]

hostname - can be either an IP address or hostname.

Options:
`, Z, Z)
	os.Stdout.Write([]byte(x))
	fs.PrintDefaults()
	os.Exit(rc)
}

// will be filled by the build script
var ProductVersion = "UNKNOWN"
var RepoVersion = "UNKNOWN"
