// pinger.go -- pinger interface

package main

import (
	"fmt"
	"time"

	logger "github.com/opencoff/go-logger"
)

type Pinger interface {
	Stop()
}

type PingOpts struct {
	Host  string
	Port  uint16
	Proto string

	Batchsize int
	Interval  time.Duration
	Timeout   time.Duration

	Logger *logger.Logger
}

type IcmpResult struct {
	Rtt time.Duration
}

type HttpsResult struct {
	DnsRtt   time.Duration
	ConnRtt  time.Duration
	TlsRtt   time.Duration
	HttpRtt  time.Duration
	HttpsRtt time.Duration
}

func (h HttpsResult) String() string {
	return fmt.Sprintf("dns: %s, tcp: %s, tls: %s, http: %s, e2e: %s",
		h.DnsRtt, h.ConnRtt, h.TlsRtt, h.HttpRtt, h.HttpsRtt)
}
