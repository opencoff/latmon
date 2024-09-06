// pinger.go -- pinger interface

package main

import (
	"time"
)

type Pinger interface {
	Ping(count int) (Result, error)
}


type PingOpts struct {
	Host	 string
	Port	 uint16
	Proto	 string
	Interval time.Duration
	Timeout  time.Duration
}

type Result struct {
	Rtt []time.Duration

	// packets lost
	Lost	int

	// duplicate receives
	Dups	int
}

