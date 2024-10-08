// https.go - https pinger
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	logger "github.com/opencoff/go-logger"
	"github.com/opencoff/latmon/internal/http"
)

type hping struct {
	PingOpts

	log logger.Logger
	url string
	cl  *http.Client
	ch  chan HttpsResult

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var _ Pinger = &hping{}

func NewHttps(cx context.Context, opts PingOpts) (*hping, chan HttpsResult, error) {
	ctx, cancel := context.WithCancel(cx)
	h := &hping{
		PingOpts: opts,
		log:      opts.Logger.New("https", 0),
		url:      fmt.Sprintf("https://%s:%d", opts.Host, opts.Port),
		cl:       http.NewClient(opts.Timeout),
		ch:       make(chan HttpsResult, 1),
		ctx:      ctx,
		cancel:   cancel,
	}

	h.log.Info("starting https pinger: %s, every %s, timeout %s", h.url, h.Interval, h.Timeout)

	h.wg.Add(1)
	go h.run()

	return h, h.ch, nil
}

func (h *hping) Stop() {
	h.cancel()
	h.wg.Wait()
	close(h.ch)
	h.log.Info("stopped https pinger: %s", h.url)
}

func (h *hping) run() {
	tick := time.NewTicker(h.Interval)
	defer func() {
		tick.Stop()
		h.wg.Done()
	}()

	done := h.ctx.Done()
	errs := 0
	for {
		select {
		case <-tick.C:
			h.log.Debug("ping %s ..", h.url)
			resp, err := h.ping()
			if err != nil {
				errs += 1
				if errs > 3 {
					h.log.Warn("%s\nToo many errors. Bailing ..", err)
					Die("http: %s; too many errors. Exiting!", h.url)
				}
				h.log.Warn("%s", err)
				continue
			}
			errs = 0
			resp.Body.Close()

			// send out measurements
			h.ch <- HttpsResult{
				DnsRtt:   resp.Dns,
				ConnRtt:  resp.Tcp,
				TlsRtt:   resp.Tls,
				HttpRtt:  resp.Http,
				HttpsRtt: resp.E2e,
			}

		case <-done:
			return
		}
	}
}

func (h *hping) ping() (*http.Response, error) {
	req := http.NewRequest("HEAD", h.url)
	req.Headers.Add("Connection", "close")

	return h.cl.Do(req, h.ctx)
}
