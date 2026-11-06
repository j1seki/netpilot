// Package db manages the local SQLite database for persistent routing rules,
// historical analytics, operation audit logs, and test run records.
package db

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Database wraps the SQLite connection and methods.
type Database struct {
	db *sql.DB
	mu sync.Mutex
}

// Open initializes the SQLite database at the specified path (or in-memory if ":memory:").
func Open(path string) (*Database, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	d := &Database{db: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}

	return d, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS rules (
		id TEXT PRIMARY KEY,
		app_name TEXT NOT NULL,
		app_path TEXT NOT NULL,
		domain_pattern TEXT NOT NULL DEFAULT '',
		preferred_interface TEXT NOT NULL,
		fallback_interface TEXT NOT NULL DEFAULT '',
		priority INTEGER NOT NULL DEFAULT 10,
		bandwidth_limit_mbps REAL NOT NULL DEFAULT 0,
		data_quota_mb REAL NOT NULL DEFAULT 0,
		data_used_mb REAL NOT NULL DEFAULT 0,
		custom_dns TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS adapter_caps (
		interface_name TEXT PRIMARY KEY,
		rx_limit_mbps REAL NOT NULL DEFAULT 0,
		tx_limit_mbps REAL NOT NULL DEFAULT 0,
		data_quota_mb REAL NOT NULL DEFAULT 0,
		data_used_mb REAL NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS monthly_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		year_month TEXT NOT NULL,
		interface_name TEXT NOT NULL,
		app_name TEXT NOT NULL DEFAULT '',
		bytes_in INTEGER NOT NULL DEFAULT 0,
		bytes_out INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(year_month, interface_name, app_name)
	);

	CREATE TABLE IF NOT EXISTS traffic_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		interface_name TEXT NOT NULL,
		app_name TEXT NOT NULL DEFAULT '',
		bytes_in INTEGER NOT NULL DEFAULT 0,
		bytes_out INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS daily_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day TEXT NOT NULL,
		interface_name TEXT NOT NULL,
		app_name TEXT NOT NULL DEFAULT '',
		bytes_in INTEGER NOT NULL DEFAULT 0,
		bytes_out INTEGER NOT NULL DEFAULT 0,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(day, interface_name, app_name)
	);

	CREATE TABLE IF NOT EXISTS operations (
		id TEXT PRIMARY KEY,
		correlation_id TEXT NOT NULL,
		parent_op_id TEXT,
		name TEXT NOT NULL,
		component TEXT NOT NULL,
		phase TEXT NOT NULL,
		platform TEXT NOT NULL,
		execution_mode TEXT NOT NULL,
		result TEXT NOT NULL,
		error_details TEXT,
		duration_ms REAL NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS test_runs (
		run_id TEXT PRIMARY KEY,
		platform TEXT NOT NULL,
		suite_name TEXT NOT NULL,
		total_tests INTEGER NOT NULL,
		passed INTEGER NOT NULL,
		failed INTEGER NOT NULL,
		skipped INTEGER NOT NULL,
		blocked INTEGER NOT NULL,
		duration_ms REAL NOT NULL,
		environment_summary TEXT NOT NULL,
		ran_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_traffic_timestamp ON traffic_samples(timestamp);
	CREATE INDEX IF NOT EXISTS idx_operations_correlation ON operations(correlation_id);
	CREATE INDEX IF NOT EXISTS idx_rules_priority ON rules(priority DESC);
	CREATE INDEX IF NOT EXISTS idx_monthly_ym ON monthly_history(year_month);
	CREATE INDEX IF NOT EXISTS idx_daily_day ON daily_history(day);
	`
	if _, err := d.db.Exec(schema); err != nil {
		return err
	}

	// Gracefully add custom_dns to existing rules table if needed
	_, _ = d.db.Exec("ALTER TABLE rules ADD COLUMN custom_dns TEXT NOT NULL DEFAULT ''")
	return nil
}

// RuleRecord represents a persistent routing rule.
type RuleRecord struct {
	ID                 string    `json:"id"`
	AppName            string    `json:"appName"`
	AppPath            string    `json:"appPath"`
	DomainPattern      string    `json:"domainPattern"`
	PreferredInterface string    `json:"preferredInterface"`
	FallbackInterface  string    `json:"fallbackInterface"`
	Priority           int       `json:"priority"`
	BandwidthLimitMbps float64   `json:"bandwidthLimitMbps"`
	DataQuotaMB        float64   `json:"dataQuotaMb"`
	DataUsedMB         float64   `json:"dataUsedMb"`
	CustomDNS          string    `json:"customDns"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// SaveRule inserts or updates a routing rule.
func (d *Database) SaveRule(r RuleRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT INTO rules (id, app_name, app_path, domain_pattern, preferred_interface, fallback_interface, priority, bandwidth_limit_mbps, data_quota_mb, data_used_mb, custom_dns, enabled, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		app_name = excluded.app_name,
		app_path = excluded.app_path,
		domain_pattern = excluded.domain_pattern,
		preferred_interface = excluded.preferred_interface,
		fallback_interface = excluded.fallback_interface,
		priority = excluded.priority,
		bandwidth_limit_mbps = excluded.bandwidth_limit_mbps,
		data_quota_mb = excluded.data_quota_mb,
		data_used_mb = excluded.data_used_mb,
		custom_dns = excluded.custom_dns,
		enabled = excluded.enabled,
		updated_at = CURRENT_TIMESTAMP;
	`
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}

	_, err := d.db.Exec(query, r.ID, r.AppName, r.AppPath, r.DomainPattern, r.PreferredInterface, r.FallbackInterface, r.Priority, r.BandwidthLimitMbps, r.DataQuotaMB, r.DataUsedMB, r.CustomDNS, enabledInt)
	return err
}

// ListRules retrieves all rules sorted by priority descending.
func (d *Database) ListRules() ([]RuleRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT id, app_name, app_path, domain_pattern, preferred_interface, fallback_interface, priority, bandwidth_limit_mbps, data_quota_mb, data_used_mb, custom_dns, enabled, created_at, updated_at
		FROM rules
		ORDER BY priority DESC, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RuleRecord
	for rows.Next() {
		var r RuleRecord
		var enabledInt int
		if err := rows.Scan(&r.ID, &r.AppName, &r.AppPath, &r.DomainPattern, &r.PreferredInterface, &r.FallbackInterface, &r.Priority, &r.BandwidthLimitMbps, &r.DataQuotaMB, &r.DataUsedMB, &r.CustomDNS, &enabledInt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt == 1
		out = append(out, r)
	}
	return out, nil
}

// DeleteRule removes a rule by ID.
func (d *Database) DeleteRule(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec("DELETE FROM rules WHERE id = ?", id)
	return err
}

// RecordTrafficSample stores traffic metrics for analytics.
func (d *Database) RecordTrafficSample(iface, app string, bytesIn, bytesOut int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(
		"INSERT INTO traffic_samples (interface_name, app_name, bytes_in, bytes_out) VALUES (?, ?, ?, ?)",
		iface, app, bytesIn, bytesOut,
	)
	return err
}

// TestRunRecord documents a platform test execution.
type TestRunRecord struct {
	RunID              string    `json:"runId"`
	Platform           string    `json:"platform"`
	SuiteName          string    `json:"suiteName"`
	TotalTests         int       `json:"totalTests"`
	Passed             int       `json:"passed"`
	Failed             int       `json:"failed"`
	Skipped            int       `json:"skipped"`
	Blocked            int       `json:"blocked"`
	DurationMs         float64   `json:"durationMs"`
	EnvironmentSummary string    `json:"environmentSummary"`
	RanAt              time.Time `json:"ranAt"`
}

// SaveTestRun records a test run.
func (d *Database) SaveTestRun(tr TestRunRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`
		INSERT INTO test_runs (run_id, platform, suite_name, total_tests, passed, failed, skipped, blocked, duration_ms, environment_summary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tr.RunID, tr.Platform, tr.SuiteName, tr.TotalTests, tr.Passed, tr.Failed, tr.Skipped, tr.Blocked, tr.DurationMs, tr.EnvironmentSummary,
	)
	return err
}

// ListTestRuns returns recent test runs.
func (d *Database) ListTestRuns(limit int) ([]TestRunRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.Query(`
		SELECT run_id, platform, suite_name, total_tests, passed, failed, skipped, blocked, duration_ms, environment_summary, ran_at
		FROM test_runs
		ORDER BY ran_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []TestRunRecord
	for rows.Next() {
		var tr TestRunRecord
		if err := rows.Scan(&tr.RunID, &tr.Platform, &tr.SuiteName, &tr.TotalTests, &tr.Passed, &tr.Failed, &tr.Skipped, &tr.Blocked, &tr.DurationMs, &tr.EnvironmentSummary, &tr.RanAt); err != nil {
			return nil, err
		}
		list = append(list, tr)
	}
	return list, nil
}

// AdapterCapRecord specifies speed limits and data quotas for an adapter.
type AdapterCapRecord struct {
	InterfaceName string    `json:"interfaceName"`
	RxLimitMbps   float64   `json:"rxLimitMbps"`
	TxLimitMbps   float64   `json:"txLimitMbps"`
	DataQuotaMB   float64   `json:"dataQuotaMb"`
	DataUsedMB    float64   `json:"dataUsedMb"`
	Enabled       bool      `json:"enabled"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// SaveAdapterCap saves or updates rate limits and quotas for an interface.
func (d *Database) SaveAdapterCap(ac AdapterCapRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT INTO adapter_caps (interface_name, rx_limit_mbps, tx_limit_mbps, data_quota_mb, data_used_mb, enabled, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(interface_name) DO UPDATE SET
		rx_limit_mbps = excluded.rx_limit_mbps,
		tx_limit_mbps = excluded.tx_limit_mbps,
		data_quota_mb = excluded.data_quota_mb,
		data_used_mb = excluded.data_used_mb,
		enabled = excluded.enabled,
		updated_at = CURRENT_TIMESTAMP;
	`
	enabledInt := 0
	if ac.Enabled {
		enabledInt = 1
	}
	_, err := d.db.Exec(query, ac.InterfaceName, ac.RxLimitMbps, ac.TxLimitMbps, ac.DataQuotaMB, ac.DataUsedMB, enabledInt)
	return err
}

// GetAdapterCaps retrieves all configured adapter limits.
func (d *Database) GetAdapterCaps() ([]AdapterCapRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT interface_name, rx_limit_mbps, tx_limit_mbps, data_quota_mb, data_used_mb, enabled, updated_at
		FROM adapter_caps
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AdapterCapRecord
	for rows.Next() {
		var ac AdapterCapRecord
		var enabledInt int
		if err := rows.Scan(&ac.InterfaceName, &ac.RxLimitMbps, &ac.TxLimitMbps, &ac.DataQuotaMB, &ac.DataUsedMB, &enabledInt, &ac.UpdatedAt); err != nil {
			return nil, err
		}
		ac.Enabled = enabledInt == 1
		list = append(list, ac)
	}
	return list, nil
}

// GetAdapterCap returns caps for a specific interface.
func (d *Database) GetAdapterCap(name string) (*AdapterCapRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var ac AdapterCapRecord
	var enabledInt int
	err := d.db.QueryRow(`
		SELECT interface_name, rx_limit_mbps, tx_limit_mbps, data_quota_mb, data_used_mb, enabled, updated_at
		FROM adapter_caps WHERE interface_name = ?
	`, name).Scan(&ac.InterfaceName, &ac.RxLimitMbps, &ac.TxLimitMbps, &ac.DataQuotaMB, &ac.DataUsedMB, &enabledInt, &ac.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ac.Enabled = enabledInt == 1
	return &ac, nil
}

// MonthlyUsageRecord captures aggregate monthly network usage.
type MonthlyUsageRecord struct {
	ID            int64     `json:"id"`
	YearMonth     string    `json:"yearMonth"`
	InterfaceName string    `json:"interfaceName"`
	AppName       string    `json:"appName"`
	BytesIn       int64     `json:"bytesIn"`
	BytesOut      int64     `json:"bytesOut"`
	RxMB          float64   `json:"rxMb"`
	TxMB          float64   `json:"txMb"`
	TotalMB       float64   `json:"totalMb"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// MonthlySummary aggregates total data transferred per month.
type MonthlySummary struct {
	YearMonth string  `json:"yearMonth"`
	TotalRxMB float64 `json:"totalRxMb"`
	TotalTxMB float64 `json:"totalTxMb"`
	TotalMB   float64 `json:"totalMb"`
	TopApp    string  `json:"topApp"`
	TopNIC    string  `json:"topNic"`
}

// RecordMonthlyUsage increments monthly accounting counters.
func (d *Database) RecordMonthlyUsage(yearMonth, iface, app string, bytesIn, bytesOut int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if yearMonth == "" {
		yearMonth = time.Now().Format("2006-01")
	}

	query := `
	INSERT INTO monthly_history (year_month, interface_name, app_name, bytes_in, bytes_out, updated_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(year_month, interface_name, app_name) DO UPDATE SET
		bytes_in = bytes_in + excluded.bytes_in,
		bytes_out = bytes_out + excluded.bytes_out,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err := d.db.Exec(query, yearMonth, iface, app, bytesIn, bytesOut)
	return err
}

// loopbackFilter excludes loopback adapters from usage totals. Loopback bytes are
// local IPC, not internet traffic; historical rows recorded before loopback was
// skipped at sampling time would otherwise inflate every "this month" number.
const loopbackFilter = `LOWER(interface_name) NOT IN ('lo','lo0') AND LOWER(interface_name) NOT LIKE 'loopback%'`

// DailyUsageRecord captures aggregate daily network usage.
type DailyUsageRecord struct {
	ID            int64     `json:"id"`
	Day           string    `json:"day"`
	InterfaceName string    `json:"interfaceName"`
	AppName       string    `json:"appName"`
	BytesIn       int64     `json:"bytesIn"`
	BytesOut      int64     `json:"bytesOut"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// RecordDailyUsage increments daily accounting counters. Keeping this table
// separate from monthly_history lets the UI answer "today" and "last 7 days"
// without rescanning raw packet samples.
func (d *Database) RecordDailyUsage(day, iface, app string, bytesIn, bytesOut int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if day == "" {
		day = time.Now().Format("2006-01-02")
	}
	query := `
	INSERT INTO daily_history (day, interface_name, app_name, bytes_in, bytes_out, updated_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(day, interface_name, app_name) DO UPDATE SET
		bytes_in = bytes_in + excluded.bytes_in,
		bytes_out = bytes_out + excluded.bytes_out,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err := d.db.Exec(query, day, iface, app, bytesIn, bytesOut)
	return err
}

// GetDailyHistory retrieves daily entries between two YYYY-MM-DD dates (inclusive).
func (d *Database) GetDailyHistory(fromDay, toDay string) ([]DailyUsageRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT id, day, interface_name, app_name, bytes_in, bytes_out, updated_at
		FROM daily_history
		WHERE day >= ? AND day <= ?
		  AND `+loopbackFilter+`
		ORDER BY day DESC, (bytes_in + bytes_out) DESC
	`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]DailyUsageRecord, 0, 16)
	for rows.Next() {
		var rec DailyUsageRecord
		if err := rows.Scan(&rec.ID, &rec.Day, &rec.InterfaceName, &rec.AppName, &rec.BytesIn, &rec.BytesOut, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, nil
}

// GetMonthlyHistory retrieves historical entries for a month or all months if empty.
func (d *Database) GetMonthlyHistory(yearMonth string) ([]MonthlyUsageRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var rows *sql.Rows
	var err error
	if yearMonth != "" && yearMonth != "all" {
		rows, err = d.db.Query(`
			SELECT id, year_month, interface_name, app_name, bytes_in, bytes_out, updated_at
			FROM monthly_history
			WHERE year_month = ?
			  AND `+loopbackFilter+`
			ORDER BY (bytes_in + bytes_out) DESC
		`, yearMonth)
	} else {
		rows, err = d.db.Query(`
			SELECT id, year_month, interface_name, app_name, bytes_in, bytes_out, updated_at
			FROM monthly_history
			WHERE ` + loopbackFilter + `
			ORDER BY year_month DESC, (bytes_in + bytes_out) DESC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MonthlyUsageRecord
	for rows.Next() {
		var rec MonthlyUsageRecord
		if err := rows.Scan(&rec.ID, &rec.YearMonth, &rec.InterfaceName, &rec.AppName, &rec.BytesIn, &rec.BytesOut, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		rec.RxMB = float64(rec.BytesIn) / (1024 * 1024)
		rec.TxMB = float64(rec.BytesOut) / (1024 * 1024)
		rec.TotalMB = rec.RxMB + rec.TxMB
		list = append(list, rec)
	}
	return list, nil
}

// GetAllMonthlySummaries returns monthly totals across all recorded months.
func (d *Database) GetAllMonthlySummaries() ([]MonthlySummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT year_month,
		       SUM(bytes_in) AS total_in,
		       SUM(bytes_out) AS total_out
		FROM monthly_history
		WHERE ` + loopbackFilter + `
		GROUP BY year_month
		ORDER BY year_month DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []MonthlySummary
	for rows.Next() {
		var s MonthlySummary
		var totalIn, totalOut int64
		if err := rows.Scan(&s.YearMonth, &totalIn, &totalOut); err != nil {
			return nil, err
		}
		s.TotalRxMB = float64(totalIn) / (1024 * 1024)
		s.TotalTxMB = float64(totalOut) / (1024 * 1024)
		s.TotalMB = s.TotalRxMB + s.TotalTxMB
		summaries = append(summaries, s)
	}
	return summaries, nil
}

// ExportMonthlyCSV formats historical records as spreadsheet-safe CSV.
// Text columns are escaped both for CSV syntax and to prevent formula execution
// when users open an export in Excel or another spreadsheet application.
func (d *Database) ExportMonthlyCSV(yearMonth string) (string, error) {
	records, err := d.GetMonthlyHistory(yearMonth)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	writer := csv.NewWriter(&out)
	writer.UseCRLF = true
	if err := writer.Write([]string{
		"YearMonth", "Interface", "Application", "BytesIn", "BytesOut",
		"RxMB", "TxMB", "TotalMB", "LastUpdated",
	}); err != nil {
		return "", err
	}

	for _, r := range records {
		row := []string{
			spreadsheetSafeCell(r.YearMonth),
			spreadsheetSafeCell(r.InterfaceName),
			spreadsheetSafeCell(r.AppName),
			fmt.Sprintf("%d", r.BytesIn),
			fmt.Sprintf("%d", r.BytesOut),
			fmt.Sprintf("%.2f", r.RxMB),
			fmt.Sprintf("%.2f", r.TxMB),
			fmt.Sprintf("%.2f", r.TotalMB),
			r.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// spreadsheetSafeCell neutralizes formula-leading characters in untrusted text
// while leaving ordinary names unchanged. csv.Writer handles delimiters, quotes,
// and embedded newlines separately.
func spreadsheetSafeCell(value string) string {
	trimmed := strings.TrimLeft(value, " \\t\\r\\n")
	if trimmed == "" {
		return value
	}
	switch {
	case strings.HasPrefix(trimmed, "="),
		strings.HasPrefix(trimmed, "+"),
		strings.HasPrefix(trimmed, "-"),
		strings.HasPrefix(trimmed, "@"),
		strings.HasPrefix(trimmed, "＝"),
		strings.HasPrefix(trimmed, "＋"),
		strings.HasPrefix(trimmed, "－"),
		strings.HasPrefix(trimmed, "＠"):
		return "'" + value
	default:
		return value
	}
}
