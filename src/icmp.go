// ping.go - pinger using ICMP

package main

import (
	"context"
	"sync"
	"time"

	ping "github.com/prometheus-community/pro-bing"
)

type icmpPinger struct {
	p      *ping.Pinger
	ch     chan IcmpResult
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

var _ Pinger = &icmpPinger{}

func NewIcmp(cx context.Context, opts PingOpts) (*icmpPinger, chan IcmpResult, error) {
	p, err := ping.NewPinger(opts.Host)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(cx)

	ip := &icmpPinger{
		p:      p,
		ch:     make(chan IcmpResult, 1),
		ctx:    ctx,
		cancel: cancel,
	}

	p.Interval = opts.Interval
	p.Timeout = opts.Timeout

	// by default we are NOT running as root
	p.SetPrivileged(false)

	// don't record RTTs, we'll process them here
	p.RecordRtts = false

	ip.wg.Add(1)
	go ip.loop()

	return ip, ip.ch, nil
}

func (ip *icmpPinger) Stop() {
	ip.cancel()
	ip.wg.Wait()
	close(ip.ch)
}

func (ip *icmpPinger) loop() {
	defer ip.wg.Done()

	p := ip.p

	maxWait := 5 * time.Second
	n := uint64(maxWait) / uint64(p.Interval)
	if n == 0 {
		n = 3
	}

	p.OnRecv = func(pkt *ping.Packet) {
		r := IcmpResult{
			Rtt: pkt.Rtt,
		}
		ip.ch <- r
	}
	p.Count = int(n)

	done := ip.ctx.Done()

	// We loop indefinitely until we're closed
	for {
		select {
		case <-done:
			return
		default:
		}

		// XXX Error??!
		if err := p.Run(); err != nil {
			err = err
		}
	}
}
