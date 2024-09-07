// https.go -- https pinger; also collects TCP/TLS/DNS traces

package main

import (
	"context"
	"fmt"
	"time"

	ping "github.com/prometheus-community/pro-bing"
)

type httpPinger struct {
	p  *ping.HTTPCaller
	ch chan HttpsResult
}

var _ Pinger = &httpPinger{}

func NewHttps(cx context.Context, opts PingOpts) (*httpPinger, chan HttpsResult, error) {
	ch := make(chan HttpsResult, 1)

	url := fmt.Sprintf("%s:%d", opts.Host, opts.Port)
	ph := ping.NewHttpCaller(url,
		ping.WithHTTPCallerMethod("HEAD"),
		ping.WithHTTPCallerCallFrequency(opts.Interval),
		ping.WithHTTPCallerTimeout(opts.Timeout),
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
