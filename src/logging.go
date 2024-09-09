// logging.go - logging infra and adapters

package main

import (
	logger "github.com/opencoff/go-logger"
	ping "github.com/prometheus-community/pro-bing"
)

type logAdapter struct {
	log *logger.Logger
}

var _ ping.Logger = &logAdapter{}

func LogAdapter(log *logger.Logger) *logAdapter {
	return &logAdapter{log}
}

// adapter methods - implements ping.Logger interface
func (a *logAdapter) Fatalf(format string, v ...interface{}) {
	a.log.Fatal(format, v...)
}

func (a *logAdapter) Errorf(format string, v ...interface{}) {
	a.log.Error(format, v...)
}

func (a *logAdapter) Warnf(format string, v ...interface{}) {
	a.log.Warn(format, v...)
}

func (a *logAdapter) Infof(format string, v ...interface{}) {
	a.log.Info(format, v...)
}

func (a *logAdapter) Debugf(format string, v ...interface{}) {
	a.log.Debug(format, v...)
}
