// plot.go - plot a html chart of latencies
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

type Rtt struct {
	Ping float64

	// raw TCP RTT
	Tcp float64

	// TLS connection setup
	Tls float64

	// http roundtrip (GET /)
	Http float64
}

func plotChart(o *outputCol, fn string) error {
	line := charts.NewLine()

	// set some global options like Title/Legend/ToolTip or anything else
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    fmt.Sprintf("RTT for %s", o.name),
			Subtitle: "Various protocol latencies",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}),
		charts.WithDataZoomOpts(opts.DataZoom{
			Type:  "slider",
			Start: float32(0),
			End:   float32(o.minlen),
		}),
	)

	line.SetXAxis(makeXAxis(o.minlen))

	for i, nm := range o.names {
		v := durationToFloat64(o.colref[i][:o.minlen])
		line.AddSeries(strings.ToTitle(nm), v)
	}

	o1 := charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(true), ShowSymbol: opts.Bool(true), SymbolSize: 5, Symbol: "diamond"})
	o2 := charts.WithMarkLineNameTypeItemOpts(
		opts.MarkLineNameTypeItem{Name: "Max", Type: "max"},
		opts.MarkLineNameTypeItem{Name: "Avg", Type: "average"},
	)

	line.SetSeriesOptions(o1, o2)

	page := components.NewPage()
	page.AddCharts(line)
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	page.Render(f)
	return nil
}

func durationToFloat64(d []time.Duration) []opts.LineData {
	f := make([]opts.LineData, len(d))
	for i, v := range d {
		f[i].Value = float64(v.Milliseconds())
	}
	return f
}

func makeXAxis(n int) []int {
	x := make([]int, n)
	for i := range n {
		x[i] = i
	}
	return x
}

/*
func plotDurations(o *outputCol, fn string) error {

	n := len(rtt)
	xaxis := make([]int, n)
	icmp := make([]opts.LineData, n)
	tcp := make([]opts.LineData, n)
	tls := make([]opts.LineData, n)
	http := make([]opts.LineData, n)
	https := make([]opts.LineData, n)

	for i := range rtt {
		o := &rtt[i]
		icmp[i].Value = o.Ping
		tcp[i].Value = o.Tcp
		tls[i].Value = o.Tls
		http[i].Value = o.Http
		https[i].Value = o.Tcp + o.Tls + o.Http
		xaxis[i] = i
	}

	line := charts.NewLine()
	// set some global options like Title/Legend/ToolTip or anything else
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    "RTT",
			Subtitle: "HTTPS, ICMP RTT latencies",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}),
		charts.WithDataZoomOpts(opts.DataZoom{
			Type:  "slider",
			Start: float32(0),
			End:   float32(n),
		}),
	)

	// Put data into instance
	line.SetXAxis(xaxis).
		AddSeries("ICMP/Ping", icmp).
		AddSeries("TCP", tcp).
		AddSeries("TLS", tls).
		AddSeries("HTTP", http).
		AddSeries("HTTPS", https)

	o1 := charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(true), ShowSymbol: opts.Bool(true), SymbolSize: 5, Symbol: "diamond"})
	o2 := charts.WithMarkLineNameTypeItemOpts(
		opts.MarkLineNameTypeItem{Name: "Max", Type: "max"},
		opts.MarkLineNameTypeItem{Name: "Avg", Type: "average"},
	)

	line.SetSeriesOptions(o1, o2)

	page := components.NewPage()
	page.AddCharts(line)
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	page.Render(f)
	return nil
}
*/
