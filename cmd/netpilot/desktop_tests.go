package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"netpilot/internal/logger"
	"netpilot/internal/testrunner"
)

type desktopTestResult struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
}

type desktopTestBridge struct {
	send    func(string)
	mu      sync.Mutex
	pending string
	result  chan bool
}

func (b *desktopTestBridge) checkSort() (testrunner.TestStatus, string) {
	return b.check("process-sort", "process sorting", "The desktop WebView verified process grouping, both network sort directions, and zero/unavailable rates")
}

func (b *desktopTestBridge) checkMetricUnits() (testrunner.TestStatus, string) {
	return b.check("metric-units", "metric unit conversion", "The desktop WebView verified decimal and binary volume conversions and bit/byte transfer rates")
}

func (b *desktopTestBridge) check(key, name, evidence string) (testrunner.TestStatus, string) {
	if b.send == nil {
		return testrunner.StatusBlocked, "Open the desktop app to check " + name
	}
	id := logger.RandomID()
	encodedID, _ := json.Marshal(id)
	result := make(chan bool, 1)
	b.mu.Lock()
	b.pending, b.result = id, result
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.pending, b.result = "", nil
		b.mu.Unlock()
	}()
	b.send("window.dispatchEvent(new CustomEvent('netpilot:test-" + key + "', {detail:" + string(encodedID) + "}));")
	select {
	case passed := <-result:
		if !passed {
			return testrunner.StatusFail, "The desktop WebView failed an assertion for " + name
		}
		return testrunner.StatusPass, evidence
	case <-time.After(4 * time.Second):
		return testrunner.StatusBlocked, "The desktop WebView did not respond to the check for " + name
	}
}

func (b *desktopTestBridge) serveResult(w http.ResponseWriter, r *http.Request) {
	var body desktopTestResult
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if !decodeAPIObject(w, r, &body) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.result == nil || body.ID != b.pending {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	select {
	case b.result <- body.Passed:
	default:
	}
	w.WriteHeader(http.StatusNoContent)
}
