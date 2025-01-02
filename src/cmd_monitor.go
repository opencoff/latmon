// cmd_monitor.go - monitor command

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

	_MonitorCmd  = "monitor"
	_MonitorDesc = "monitor one or more https URLs and record latency"
)

type monitorCmd struct{}

func (c *monitorCmd) Name() string {
	return _MonitorCmd
}

func (c *monitorCmd) Desc() string {
	return _MonitorDesc
}

func (c *monitorCmd) Run(args []string) error {
	var interval, timeout time.Duration
	var help bool
	var dir, logdest, lvl string
	var bsz int

	fs := pflag.NewFlagSet(_MonitorCmd, pflag.ExitOnError)
	fs.DurationVarP(&interval, "every", "i", 2*time.Second, "Send pings every `I` interval apart")
	fs.IntVarP(&bsz, "batch-size", "b", _DefaultBatchSize, "Collect 'B' samples per measurement run")
	fs.DurationVarP(&timeout, "timeout", "t", 2*time.Second, "Set rx deadline to `T` seconds")
	fs.BoolVarP(&help, "help", "h", false, "Show this help message and exit")
	fs.StringVarP(&dir, "output-dir", "d", ".", "Put charts in directory `D`")
	fs.StringVarP(&logdest, "log", "L", "SYSLOG", "Send logs to destination `L`")
	fs.StringVarP(&lvl, "log-level", "", "INFO", "Log at priority `P`")

	err := fs.Parse(args)
	if err != nil {
		return err
	}

	if help {
		x := fmt.Sprintf(`%s: %s

Usage: %s [options] HOST [HOST..]

Where HOST is of the form:

	https:hostname[:port]

hostname - can be either an IP address or hostname.

Options:
`, _MonitorCmd, _MonitorDesc, _MonitorCmd)

		os.Stdout.Write([]byte(x))
		fs.PrintDefaults()
		return nil
	}

	// samples per day can't be smaller than batchsize
	perDay := int((86400 * time.Second) / interval)
	if bsz >= perDay {
		return fmt.Errorf("batch-size is greater than total samples per day (%d)", perDay)
	}

	args = fs.Args()
	if len(args) < 0 {
		return fmt.Errorf("monitor: insufficient args")
	}

	prio, ok := logger.ToPriority(lvl)
	if !ok {
		return fmt.Errorf("Unknown log level '%s'", lvl)
	}

	log, err := logger.NewLogger(logdest, prio, Z, logger.Ldate|logger.Ltime|logger.Lmicroseconds|logger.Lfileloc)
	if err != nil {
		return fmt.Errorf("can't create logger: %w", err)
	}

	log.Info("Starting latency monitor [%s, %s]; batchsize=%d interval=%s timeout=%s",
		ProductVersion, RepoVersion, bsz, interval, timeout)

	m := NewMeasurer(WithOutputDir(dir), WithBatchSize(bsz), WithLogger(log))
	defer m.Stop()

	ctx := context.Background()
	seen := make(map[string]bool)
	for _, a := range args {
		proto, host, port, err := parsePinger(a)
		if err != nil {
			return fmt.Errorf("can't create pinger: %w", err)
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
				return fmt.Errorf("can't create https pinger: %w", err)
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

	return nil
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

var _ Cmd = &monitorCmd{}

func init() {
	registerCmd(&monitorCmd{})
}
