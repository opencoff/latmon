// icmp.go - pinger using ICMP


package main

import (
	icmp "github.com/prometheus-community/pro-bing"
)


type icmpPinger struct {
	p *icmp.Pinger
}

var _ Pinger = &icmpPinger{}


func NewICMP(opts PingOpts) (*icmpPinger, error) {

	p, err := icmp.NewPinger(opts.Host)
	if err != nil {
		return nil, err
	}

	p.Interval = opts.Interval
	p.Timeout = opts.Timeout

	// by default we are NOT running as root
	p.SetPrivileged(false)

	ip := &icmpPinger{
		p: p,
	}

	return ip, nil
}


func (ip *icmpPinger) Ping(count int) (Result, error) {

	ip.p.Count = count
	if err := ip.p.Run(); err != nil {
		return Result{}, err
	}

	st := ip.p.Statistics()

	r := Result{
		Rtt: st.Rtts,
		Lost: ip.p.PacketsSent - ip.p.PacketsRecv,
		Dups: st.PacketsRecvDuplicates,
	}
	return r, nil
}
