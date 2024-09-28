// ping.go - pinger using ICMP

package main

import (
	"context"
	"errors"
	"sync"

	logger "github.com/opencoff/go-logger"
	ping "github.com/prometheus-community/pro-bing"
)

type icmpPinger struct {
	p      *ping.Pinger
	ch     chan IcmpResult
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	log    *logger.Logger
}

var _ Pinger = &icmpPinger{}

func NewIcmp(cx context.Context, opts PingOpts) (*icmpPinger, chan IcmpResult, error) {
	p, err := ping.NewPinger(opts.Host)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(cx)

	log := opts.Logger.New("icmp", 0)

	ip := &icmpPinger{
		p:      p,
		ch:     make(chan IcmpResult, 1),
		ctx:    ctx,
		cancel: cancel,
		log:    log,
	}

	p.Interval = opts.Interval
	p.SetLogger(LogAdapter(log))

	// by default we are NOT running as root
	p.SetPrivileged(false)

	// don't record RTTs, we'll process them here
	p.RecordRtts = false

	ip.log.Info("starting icmp pinger: %s, every %s, timeout %s",
		opts.Host, opts.Interval, opts.Timeout)

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
	p.OnRecv = func(pkt *ping.Packet) {
		r := IcmpResult{
			Rtt: pkt.Rtt,
		}
		ip.ch <- r
	}

	if err := p.RunWithContext(ip.ctx); err != nil && !errors.Is(err, context.Canceled) {
		ip.log.Warn("ICMP runner: %s", err)
	}
}
