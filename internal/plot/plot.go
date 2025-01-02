// plot.go - plot a html chart of latencies
package plot

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

type Columns struct {
	Name  string
	Start time.Time

	Names  []string
	Colref [][]time.Duration
	Minlen int
}

func Chart(o *Columns, wr io.Writer) error {
	//legend := makeLegend(o)

	line := charts.NewLine()
	scatter := charts.NewScatter()
	title := fmt.Sprintf("RTT for %s", o.Name)

	// set some global options like Title/Legend/ToolTip or anything else
	glopts := []charts.GlobalOpts{
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    title,
			Subtitle: fmt.Sprintf("Start: %s", o.Start),
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "item"}),
		charts.WithDataZoomOpts(opts.DataZoom{
			Type:  "slider",
			Start: float32(0),
			End:   float32(o.Minlen),
		}),
		/*
			charts.WithXAxisOpts(opts.XAxis{Name: "sample #"}),
			charts.WithYAxisOpts(opts.YAxis{Name: "latency (ms)"}),
			charts.WithToolboxOpts(opts.Toolbox{
				Show:  opts.Bool(true),
				Right: "20%",
				Feature: &opts.ToolBoxFeature{
					SaveAsImage: &opts.ToolBoxFeatureSaveAsImage{
						Show:  opts.Bool(true),
						Type:  "png",
						Title: "Save as PNG",
					},
				},
			}),
		*/
		charts.WithInitializationOpts(opts.Initialization{
			Width:  "1200px",
			Height: "800px",
		}),
	}

	line.SetGlobalOptions(glopts...)
	scatter.SetGlobalOptions(glopts...)

	xa := makeXAxis(o.Minlen)
	line.SetXAxis(xa)
	scatter.SetXAxis(xa)

	o1 := charts.WithLineChartOpts(opts.LineChart{Smooth: opts.Bool(true), ShowSymbol: opts.Bool(true), SymbolSize: 5, Symbol: "diamond"})
	o2 := charts.WithMarkLineNameTypeItemOpts(
		opts.MarkLineNameTypeItem{Name: "Max", Type: "max"},
		opts.MarkLineNameTypeItem{Name: "Avg", Type: "average"},
	)

	line.SetSeriesOptions(o1, o2)

	nscatter := 0
	for i, nm := range o.Names {
		ld, sd, n := durationToFloat64(i, o.Colref[i][:o.Minlen])
		title := strings.ToTitle(nm)
		line.AddSeries(title, ld, o1, o2)
		if n > 0 {
			nscatter++
			scatter.AddSeries(fmt.Sprintf("%s failures", title), sd)
		}
	}

	// scatter.SetXAxis(??)

	page := components.NewPage()
	page.AddCharts(line)
	page.SetPageTitle(title)
	if nscatter > 0 {
		page.AddCharts(scatter)
	}
	page.Render(wr)
	return nil
}

func durationToFloat64(idx int, d []time.Duration) ([]opts.LineData, []opts.ScatterData, int) {
	f := make([]opts.LineData, len(d))
	s := make([]opts.ScatterData, len(d))
	nscatter := 0
	for i, v := range d {
		if int64(v) > 0 {
			f[i].Value = float64(v.Milliseconds())
			f[i].XAxisIndex = idx
		} else {
			nscatter++
			f[i].Value = -1.0
			s[i] = opts.ScatterData{
				Value: -1.0,
				//Symbol: "circle",
				SymbolSize: 4,
				XAxisIndex: idx,
			}
		}
	}
	return f, s, nscatter
}

func makeXAxis(n int) []int {
	x := make([]int, n)
	for i := range n {
		x[i] = i
	}
	return x
}

func makeLegend(o *Columns) []string {
	var v []string

	v = append(v, fmt.Sprintf("Start: %s", o.Start))
	for i, col := range o.Colref {
		z := 0
		for _, d := range col {
			if int64(d) < 0 {
				z++
			}
		}
		if z > 0 {
			v = append(v, fmt.Sprintf("%s failures: %d", o.Names[i], z))
		}
	}
	return v
}

/*
func plotDurations(o *Columns, fn string) error {

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
