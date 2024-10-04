// https.go -- https pinger; also collects TCP/TLS/DNS traces

package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ping "github.com/prometheus-community/pro-bing"
)

type httpPinger struct {
	p  *ping.HTTPCaller
	ch chan HttpsResult
}

var _ Pinger = &httpPinger{}

func NewHttpsOld(cx context.Context, opts PingOpts) (*httpPinger, chan HttpsResult, error) {
	ch := make(chan HttpsResult, 1)

	log := opts.Logger.New("https", 0)

	// We will create a transport that always creates a new socket, dns resolution etc.
	// This way we will capture all latencies (the default behavior of http.Client.Transport
	// is to cache DNS lookups and per-host connections).
	tr := http.Transport{
		DisableKeepAlives:   true,
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		MaxConnsPerHost:     1,
		IdleConnTimeout:     1 * time.Millisecond,
	}

	cl := http.Client{
		Transport: &tr,
	}
	url := fmt.Sprintf("https://%s:%d", opts.Host, opts.Port)
	ph := ping.NewHttpCaller(url,
		ping.WithHTTPCallerMethod("HEAD"),
		ping.WithHTTPCallerCallFrequency(opts.Interval),
		ping.WithHTTPCallerClient(&cl),
		ping.WithHTTPCallerTimeout(opts.Timeout),
		ping.WithHTTPCallerLogger(LogAdapter(log)),
		ping.WithHTTPCallerOnResp(func(s *ping.TraceSuite, _ *ping.HTTPCallInfo) {
			r := HttpsResult{
				DnsRtt:   getDnsRtt(s),
				ConnRtt:  getTcpRtt(s),
				TlsRtt:   getTlsRtt(s),
				HttpRtt:  getHttpRtt(s),
				HttpsRtt: getE2eRtt(s),
			}
			ch <- r
		}),
	)

	log.Info("starting https pinger: %s, every %s, timeout %s",
		opts.Host, opts.Interval, opts.Timeout)

	go func() {
		ph.RunWithContext(cx)
	}()

	h := &httpPinger{
		p:  ph,
		ch: ch,
	}
	return h, h.ch, nil
}

func (h *httpPinger) Stop() {
	h.p.Stop()
	close(h.ch)
}

func getDnsRtt(s *ping.TraceSuite) time.Duration {
	return s.GetDNSEnd().Sub(s.GetDNSStart())
}

func getTcpRtt(s *ping.TraceSuite) time.Duration {
	return s.GetConnEnd().Sub(s.GetConnStart())
}

func getTlsRtt(s *ping.TraceSuite) time.Duration {
	return s.GetTLSEnd().Sub(s.GetTLSStart())
}

func getHttpRtt(s *ping.TraceSuite) time.Duration {
	return s.GetFirstByteReceived().Sub(s.GetWroteHeaders())
}

func getE2eRtt(s *ping.TraceSuite) time.Duration {
	return s.GetGeneralEnd().Sub(s.GetGeneralStart())
}
