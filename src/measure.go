// measure.go - measure latency and plot charts


package main

import (
	"context"
)

type Measure struct {
	MeasureOpts

}


type MeasureOpts struct {
	// number of samples per batch
	BatchSize int

	Output    string
}

func NewMeasurer(opt MeasureOpts) (*Measure, error) {
	m := &Measure{
		MeasureOpts: opt,
	}

	return m, nil
}


// Start measurement loop along with a context for cancellation
func (m *Measure) Start(ctx context.Context, p []Pinger) *Work {
	return &Work{}
}

// Stop the measurement loop
func (m *Measure) Stop(w *Work) error {
	return nil
}


func (w *Work) worker(p Pinger) {
	count := w.m.BatchSize
	for {
		r, err := p.Ping(count)
		if err != nil {
			w.errch <- err
		}
		r = r
	}
}


type Work struct {
	m *Measure
	p []Pinger
	ctx context.Context
	cancel context.CancelFunc

	// other things?
	errch chan error
}
