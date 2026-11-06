// Package analytics tracks per-adapter and per-application bandwidth utilization,
// computes rolling rates, and exports time-series statistics.
package analytics

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"netpilot/internal/db"
	"netpilot/internal/nics"
)

// NICMetric holds current rates and total byte counts for an adapter.
type NICMetric struct {
	Name            string    `json:"name"`
	BytesIn         int64     `json:"bytesIn"`
	BytesOut        int64     `json:"bytesOut"`
	CurrentRxMbps   float64   `json:"currentRxMbps"`
	CurrentTxMbps   float64   `json:"currentTxMbps"`
	PeakRxMbps      float64   `json:"peakRxMbps"`
	PeakTxMbps      float64   `json:"peakTxMbps"`
	LastSampleTime  time.Time `json:"lastSampleTime"`
}

// AppMetric holds usage statistics for a specific application.
type AppMetric struct {
	AppName         string    `json:"appName"`
	BytesIn         int64     `json:"bytesIn"`
	BytesOut        int64     `json:"bytesOut"`
	CurrentRxMbps   float64   `json:"currentRxMbps"`
	CurrentTxMbps   float64   `json:"currentTxMbps"`
	LastActive      time.Time `json:"lastActive"`
}

// Collector records bandwidth samples and computes rolling throughput.
type Collector struct {
	mu         sync.RWMutex
	database   *db.Database
	nicStats   map[string]*NICMetric
	appStats   map[string]*AppMetric
	history    []TimeSeriesPoint
	maxHistory int
}

// TimeSeriesPoint represents a snapshot across all adapters for graphing.
type TimeSeriesPoint struct {
	Timestamp   time.Time          `json:"timestamp"`
	TotalRxMbps float64            `json:"totalRxMbps"`
	TotalTxMbps float64            `json:"totalTxMbps"`
	ByInterface map[string]float64 `json:"byInterface"` // interface -> total Mbps
}

// NewCollector initializes the analytics engine.
func NewCollector(database *db.Database) *Collector {
	return &Collector{
		database:   database,
		nicStats:   make(map[string]*NICMetric),
		appStats:   make(map[string]*AppMetric),
		maxHistory: 120, // 2 minutes at 1s intervals
	}
}

// RecordNICSample updates byte counters and computes instant Mbps.
func (c *Collector) RecordNICSample(name string, totalIn, totalOut int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	m, ok := c.nicStats[name]
	if !ok {
		c.nicStats[name] = &NICMetric{
			Name:           name,
			BytesIn:        totalIn,
			BytesOut:       totalOut,
			LastSampleTime: now,
		}
		return
	}

	deltaSec := now.Sub(m.LastSampleTime).Seconds()
	if deltaSec <= 0.05 {
		return // Too frequent
	}

	deltaIn := totalIn - m.BytesIn
	deltaOut := totalOut - m.BytesOut

	if deltaIn < 0 {
		deltaIn = 0 // Interface reset
	}
	if deltaOut < 0 {
		deltaOut = 0
	}

	rxMbps := (float64(deltaIn*8) / deltaSec) / 1000000.0
	txMbps := (float64(deltaOut*8) / deltaSec) / 1000000.0

	m.BytesIn = totalIn
	m.BytesOut = totalOut
	m.CurrentRxMbps = rxMbps
	m.CurrentTxMbps = txMbps
	m.LastSampleTime = now

	if rxMbps > m.PeakRxMbps {
		m.PeakRxMbps = rxMbps
	}
	if txMbps > m.PeakTxMbps {
		m.PeakTxMbps = txMbps
	}
	// Record positive byte deltas to SQLite monthly and daily history so Usage &
	// History can show this month, last week and today from real counters.
	if c.database != nil && (deltaIn > 0 || deltaOut > 0) {
		_ = c.database.RecordMonthlyUsage("", name, "system", deltaIn, deltaOut)
		_ = c.database.RecordDailyUsage("", name, "system", deltaIn, deltaOut)
	}
}
func (c *Collector) RecordAppUsage(appName string, bytesIn, bytesOut int64, durationSec float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	am, ok := c.appStats[appName]
	if !ok {
		am = &AppMetric{
			AppName: appName,
		}
		c.appStats[appName] = am
	}

	am.BytesIn += bytesIn
	am.BytesOut += bytesOut
	am.LastActive = time.Now()

	if durationSec > 0 {
		am.CurrentRxMbps = (float64(bytesIn*8) / durationSec) / 1000000.0
		am.CurrentTxMbps = (float64(bytesOut*8) / durationSec) / 1000000.0
	}
}

// Snapshot returns current metrics for UI display.
func (c *Collector) Snapshot() ([]NICMetric, []AppMetric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nics := make([]NICMetric, 0, len(c.nicStats))
	for _, m := range c.nicStats {
		nics = append(nics, *m)
	}

	apps := make([]AppMetric, 0, len(c.appStats))
	for _, a := range c.appStats {
		apps = append(apps, *a)
	}

	return nics, apps
}

// PushHistoryPoint adds a combined snapshot for live graphs.
func (c *Collector) PushHistoryPoint() TimeSeriesPoint {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	totalRx := 0.0
	totalTx := 0.0
	byNic := make(map[string]float64)

	for name, m := range c.nicStats {
		totalRx += m.CurrentRxMbps
		totalTx += m.CurrentTxMbps
		byNic[name] = m.CurrentRxMbps + m.CurrentTxMbps
	}

	point := TimeSeriesPoint{
		Timestamp:   now,
		TotalRxMbps: totalRx,
		TotalTxMbps: totalTx,
		ByInterface: byNic,
	}

	c.history = append(c.history, point)
	if len(c.history) > c.maxHistory {
		c.history = c.history[len(c.history)-c.maxHistory:]
	}

	return point
}

// GetHistory returns time-series history points.
func (c *Collector) GetHistory() []TimeSeriesPoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]TimeSeriesPoint, len(c.history))
	copy(out, c.history)
	return out
}

// SampleHostNICs samples interface byte counters using OS-specific mechanisms across Linux, Windows, and macOS.
func (c *Collector) SampleHostNICs() error {
	if runtime.GOOS == "linux" {
		return c.SampleLinuxNetDev()
	} else if runtime.GOOS == "windows" {
		return c.sampleWindowsNet()
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if in, out, ok := nics.Octets(ifc.Index); ok {
			c.RecordNICSample(ifc.Name, int64(in), int64(out))
		}
	}
	c.PushHistoryPoint()
	return nil
}

// SampleLinuxNetDev reads real interface statistics directly from /proc/net/dev on Linux.
func (c *Collector) SampleLinuxNetDev() error {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip the two header lines
	if !scanner.Scan() || !scanner.Scan() {
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue // loopback traffic is local, not internet usage
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rxBytes, errRx := strconv.ParseInt(fields[0], 10, 64)
		txBytes, errTx := strconv.ParseInt(fields[8], 10, 64)
		if errRx == nil && errTx == nil {
			c.RecordNICSample(iface, rxBytes, txBytes)
		}
	}
	c.PushHistoryPoint()
	return nil
}
