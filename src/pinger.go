// pinger.go -- pinger interface

package main

import (
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
