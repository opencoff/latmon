// measurer.go -- measurement on a per-host basis

package main

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	logger "github.com/opencoff/go-logger"
	"github.com/opencoff/latmon/internal/plot"
)

type MeasureOpt func(o *measureOpt)

func WithOutputDir(nm string) MeasureOpt {
	return func(o *measureOpt) {
		o.outdir = nm
	}
}

func WithBatchSize(sz int) MeasureOpt {
	return func(o *measureOpt) {
		o.batchsize = sz
	}
}

func WithLogger(log *logger.Logger) MeasureOpt {
	return func(o *measureOpt) {
		o.log = log
	}
}

type measureOpt struct {
	outdir    string
	batchsize int
	log       *logger.Logger
}

type Measurer struct {
	measureOpt

	wg      sync.WaitGroup
	perHost map[string]*hostStats
	pingers []Pinger
}

func NewMeasurer(opts ...MeasureOpt) *Measurer {
	m := &Measurer{
		perHost: make(map[string]*hostStats),
		pingers: make([]Pinger, 0, 8),
	}

	opt := &m.measureOpt
	for _, fp := range opts {
		fp(opt)
	}

	return m
}

func (m *Measurer) AddIcmp(host string, p Pinger, ich chan IcmpResult) {
	hst, ok := m.perHost[host]
	if !ok {
		hst = newHost(host, m.batchsize)
		m.perHost[host] = hst
	}

	m.log.Debug("%s: added icmp pinger ..", host)

	// start a runner to harvest results
	m.pingers = append(m.pingers, p)
	m.wg.Add(1)
	go m.icmpWorker(hst, p, ich)
}

func (m *Measurer) AddHttps(host string, p Pinger, hch chan HttpsResult) {
	hst, ok := m.perHost[host]
	if !ok {
		hst = newHost(host, m.batchsize)
		m.perHost[host] = hst
	}

	m.log.Debug("%s: added https pinger ..", host)

	// start a runner to harvest results
	m.pingers = append(m.pingers, p)
	m.wg.Add(1)
	go m.httpsWorker(hst, p, hch)
}

func (m *Measurer) Stop() {
	m.log.Info("stopping measurements ..")

	// first stop all the individual pingers
	for _, p := range m.pingers {
		p.Stop()
	}

	// now wait for workers to complete
	m.wg.Wait()
}

// captures all proto rtt for a given host
type hostStats struct {
	sync.Mutex

	name  string
	start time.Time

	icmp []time.Duration

	dns   []time.Duration
	tcp   []time.Duration
	tls   []time.Duration
	http  []time.Duration
	https []time.Duration
}

func newHost(nm string, bsz int) *hostStats {
	m := &hostStats{
		name:  nm,
		start: time.Now().UTC(),
		icmp:  make([]time.Duration, 0, bsz),
		dns:   make([]time.Duration, 0, bsz),
		tcp:   make([]time.Duration, 0, bsz),
		tls:   make([]time.Duration, 0, bsz),
		http:  make([]time.Duration, 0, bsz),
		https: make([]time.Duration, 0, bsz),
	}
	return m
}


// flush this batch to disk and generate the charts
func (m *Measurer) flush(hs *hostStats) {
	// first gather the data and do the rest in an async way
	o := hs.makeOutput()

	// reset the start
	hs.start = time.Now().UTC()

	go m.asyncFlush(&o)
}

// asynchronously flush data and generate charts
func (m *Measurer) asyncFlush(o *plot.Columns) {
	fname := o.Start.Format("2006-01-02-15.04.05")
	stdir := path.Join(m.outdir, "stats", o.Name)
	chdir := path.Join(m.outdir, "charts", o.Name)

	stname := path.Join(stdir, fmt.Sprintf("%s.csv", fname))
	chname := path.Join(chdir, fmt.Sprintf("%s.html", fname))

	err := os.MkdirAll(stdir, 0750)
	if err != nil {
		m.log.Warn("mkdir %s: %s", stdir, err)
		return
	}

	err = os.MkdirAll(chdir, 0750)
	if err != nil {
		m.log.Warn("mkdir %s: %s", chdir, err)
		return
	}

	// first write the telemetry
	fd, err := os.OpenFile(stname, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		m.log.Warn("can't create %s: %s", stname, err)
	}

	m.log.Info("flush: %s: [%s] %d samples [cols: %s]", o.Name, fname, o.Minlen, strings.Join(o.Names, ","))
	m.log.Debug("flush: %s: raw data: %s, chart: %s", o.Name, stname, chname)

	fmt.Fprintf(fd, "%s\n", strings.Join(o.Names, ","))

	// iterate over all rows and write the raw nanosecond-granularity measurement
	z := make([]string, len(o.Names))
	for i := 0; i < o.Minlen; i++ {
		for j, col := range o.Colref {
			z[j] = fmt.Sprintf("%d", col[i])
		}
		fmt.Fprintf(fd, "%s\n", strings.Join(z, ","))
	}
	fd.Close()

	// now plot and save the chart
	if err = plot.Chart(o, chname); err != nil {
		m.log.Warn("can't create chart %s: %s", chname, err)
	}
}

func (h *hostStats) makeOutput() plot.Columns {
	o := plot.Columns{
		Name:   h.name,
		Start:  h.start,
		Minlen: 10000000000,
	}

	// we store a ref to each of the slices and create new slices.
	// This way, we can do the flush in an async goroutine and unblock the calling
	// workers
	if len(h.icmp) > 0 {
		o.Names = append(o.Names, "icmp")
		o.Colref = append(o.Colref, h.icmp)
		o.Minlen = min(o.Minlen, len(h.icmp))
		h.icmp = make([]time.Duration, 0, cap(h.icmp))
	}
	if len(h.dns) > 0 {
		o.Names = append(o.Names, "dns")
		o.Colref = append(o.Colref, h.dns)
		o.Minlen = min(o.Minlen, len(h.dns))
		h.dns = make([]time.Duration, 0, cap(h.dns))
	}
	if len(h.tcp) > 0 {
		o.Names = append(o.Names, "tcp")
		o.Colref = append(o.Colref, h.tcp)
		o.Minlen = min(o.Minlen, len(h.tcp))
		h.tcp = make([]time.Duration, 0, cap(h.tcp))
	}
	if len(h.tls) > 0 {
		o.Names = append(o.Names, "tls")
		o.Colref = append(o.Colref, h.tls)
		o.Minlen = min(o.Minlen, len(h.tls))
		h.tls = make([]time.Duration, 0, cap(h.tls))
	}
	if len(h.http) > 0 {
		o.Names = append(o.Names, "http")
		o.Colref = append(o.Colref, h.http)
		o.Minlen = min(o.Minlen, len(h.http))
		h.http = make([]time.Duration, 0, cap(h.http))
	}
	if len(h.https) > 0 {
		o.Names = append(o.Names, "https")
		o.Colref = append(o.Colref, h.https)
		o.Minlen = min(o.Minlen, len(h.https))
		h.https = make([]time.Duration, 0, cap(h.https))
	}

	return o
}

func (m *Measurer) icmpWorker(hs *hostStats, p Pinger, ich chan IcmpResult) {
	i := 0
	for r := range ich {
		i++
		m.log.Debug("icmp: %d: %s\n", i, r.Rtt)

		hs.Lock()
		if len(hs.icmp) == m.batchsize {
			i = 0
			m.flush(hs)
		}
		hs.icmp = append(hs.icmp, r.Rtt)
		hs.Unlock()
	}
	m.wg.Done()
}

func (m *Measurer) httpsWorker(hs *hostStats, p Pinger, hch chan HttpsResult) {
	i := 0
	for r := range hch {
		i++
		hs.Lock()

		m.log.Debug("https: %d: %s\n", i, r)

		if len(hs.https) == m.batchsize {
			i = 0
			m.flush(hs)
		}

		hs.dns = append(hs.dns, r.DnsRtt)
		hs.tcp = append(hs.tcp, r.ConnRtt)
		hs.tls = append(hs.tls, r.TlsRtt)
		hs.http = append(hs.http, r.HttpRtt)
		hs.https = append(hs.https, r.HttpsRtt)
		hs.Unlock()
	}
	m.wg.Done()
}
