// measurer.go -- measurement on a per-host basis

// Measurements are written to disk in "batches" ("batchsize")
// And charts are generated daily.

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

func WithLogger(log logger.Logger) MeasureOpt {
	return func(o *measureOpt) {
		o.log = log
	}
}

func WithInterval(ii time.Duration) MeasureOpt {
	return func(o *measureOpt) {
		if ii > 0 {
			o.interval = ii
		}
	}
}

type measureOpt struct {
	outdir    string
	batchsize int
	interval  time.Duration
	log       logger.Logger
}

type Measurer struct {
	measureOpt

	wg           sync.WaitGroup
	perHost      map[string]*hostStats
	perHostDaily map[string]*plot.Columns
	pingers      []Pinger
}

func NewMeasurer(opts ...MeasureOpt) *Measurer {
	m := &Measurer{
		measureOpt: measureOpt{
			outdir:    "/tmp/latmon",
			batchsize: 3600,
			interval:  2 * time.Second,
		},
		perHost:      make(map[string]*hostStats),
		perHostDaily: make(map[string]*plot.Columns),
		pingers:      make([]Pinger, 0, 8),
	}

	opt := &m.measureOpt
	for _, fp := range opts {
		fp(opt)
	}

	if m.log == nil {
		var err error
		m.log, err = logger.NewLogger("NONE", logger.LOG_INFO, "latmon", 0)
		if err != nil {
			panic("can't create empty logger")
		}
	}

	return m
}

func (m *Measurer) AddHttps(host string, p Pinger, hch chan HttpsResult) error {
	stdir := path.Join(m.outdir, "stats", host)
	chdir := path.Join(m.outdir, "charts", host)
	err := os.MkdirAll(stdir, 0750)
	if err != nil {
		return fmt.Errorf("https: mkdir: %s: %w", stdir, err)
	}

	err = os.MkdirAll(chdir, 0750)
	if err != nil {
		return fmt.Errorf("https: mkdir: %s: %w", chdir, err)
	}

	hst, ok := m.perHost[host]
	if !ok {
		hst = m.newHost(host, stdir, chdir)
	}

	m.log.Debug("%s: added https pinger ..", host)

	// start a runner to harvest results
	m.pingers = append(m.pingers, p)
	m.wg.Add(1)
	go m.httpsWorker(hst, p, hch)

	return nil
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

	statsDir string
	chartDir string

	dns   []time.Duration
	tcp   []time.Duration
	tls   []time.Duration
	http  []time.Duration
	https []time.Duration
}

func (m *Measurer) newHost(nm, stats, charts string) *hostStats {
	bsz := m.batchsize
	h := &hostStats{
		name:     nm,
		start:    time.Now().UTC(),
		statsDir: stats,
		chartDir: charts,
		dns:      make([]time.Duration, 0, bsz),
		tcp:      make([]time.Duration, 0, bsz),
		tls:      make([]time.Duration, 0, bsz),
		http:     make([]time.Duration, 0, bsz),
		https:    make([]time.Duration, 0, bsz),
	}

	m.perHost[nm] = h
	return h
}

// asynchronously flush data and generate charts
func (m *Measurer) asyncFlush(o *plot.Columns, hs *hostStats) {
	fname := o.Start.Format("2006-01-02-15.04.05")
	stname := path.Join(hs.statsDir, fmt.Sprintf("%s.csv", fname))
	chname := path.Join(hs.chartDir, fmt.Sprintf("%s.html", fname))

	m.log.Info("batch-flush: %s: [%s] %d samples [cols: %s]", o.Name, fname, o.Minlen, strings.Join(o.Names, ","))
	m.log.Debug("batch-flush: %s: raw data: %s, chart: %s", o.Name, stname, chname)

	if err := writeCharts(o, stname, chname); err != nil {
		m.log.Warn("%s", err)
	}

	// now update the daily stats and see if we need to flush it as well
	m.updateDailyStats(o, hs)
	return
}

// write telemetry and charts for 'o'
func writeCharts(o *plot.Columns, stname, chname string) error {
	// first write the telemetry/stats
	fd, err := os.OpenFile(stname, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		return fmt.Errorf("create %s: %s", stname, err)
	}

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
		return fmt.Errorf("create chart %s: %w", chname, err)
	}
	return nil
}

func (m *Measurer) updateDailyStats(o *plot.Columns, hs *hostStats) {
	ds, ok := m.perHostDaily[hs.name]
	if !ok {
		ds = &plot.Columns{
			Name:   o.Name,
			Start:  o.Start,
			Names:  o.Names,
			Colref: make([][]time.Duration, len(o.Names)),
		}
		m.perHostDaily[hs.name] = ds
	}

	perDay := int((86400 * time.Second) / m.interval)
	minlen := perDay * 10000
	for i := range o.Names {
		col := ds.Colref[i]
		if cap(col) < perDay {
			col = make([]time.Duration, 0, perDay)
		}
		col = append(col, o.Colref[i]...)
		minlen = min(minlen, len(col))
		ds.Colref[i] = col
	}

	ds.Minlen = minlen
	if len(ds.Colref[0]) < perDay {
		return
	}

	// time to flush this daily accumulator

	fname := ds.Start.Format("2006-01-02")
	stname := path.Join(hs.statsDir, fmt.Sprintf("%s.csv", fname))
	chname := path.Join(hs.chartDir, fmt.Sprintf("%s.html", fname))

	m.log.Info("daily-flush: %s: [%s] %d samples [cols: %s]", ds.Name, fname, ds.Minlen, strings.Join(ds.Names, ","))
	m.log.Debug("daily-flush: %s: raw data: %s, chart: %s", ds.Name, stname, chname)

	if err := writeCharts(ds, stname, chname); err != nil {
		m.log.Warn("%s", err)
	}

	// reset the daily counters
	for i := range o.Names {
		ds.Colref[i] = ds.Colref[i][:0]
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

	// reset the counter
	h.start = time.Now().UTC()

	return o
}

// flush this batch to disk and generate the charts
// This is called with the lock (on hs) held. Thus, we need to
// do this part quickly
func (m *Measurer) flush(hs *hostStats) {
	o := hs.makeOutput()
	go m.asyncFlush(&o, hs)
}

func (m *Measurer) httpsWorker(hs *hostStats, p Pinger, hch chan HttpsResult) {
	for r := range hch {
		hs.Lock()
		if len(hs.https) == m.batchsize {
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
