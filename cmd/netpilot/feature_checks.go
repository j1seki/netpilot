package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/engine"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
	"netpilot/internal/netmerge"
	"netpilot/internal/nics"
	"netpilot/internal/notify"
	"netpilot/internal/profiles"
	"netpilot/internal/rules"
	"netpilot/internal/state"
	"netpilot/internal/testrunner"
)

func checkAdapterQuota() (testrunner.TestStatus, string) {
	database, err := db.Open(":memory:")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer database.Close()
	svc := NetPilotService{database: database}
	post := func(body string) error {
		reply := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/adapter-caps/quota", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		svc.ServeHTTP(reply, req)
		if reply.Code != http.StatusOK {
			return fmt.Errorf("quota request returned %d: %s", reply.Code, reply.Body)
		}
		return nil
	}
	const adapter = "fixture"
	if err := post(`{"adapter":"fixture","quotaMb":10}`); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	if err := database.AddAdapterUsage(adapter, 11); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	if err := post(`{"adapter":"fixture","quotaMb":12}`); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	before, err := database.GetAdapterCap(adapter)
	if err != nil || before == nil || before.DataQuotaMB != 12 || before.DataUsedMB != 11 {
		return testrunner.StatusFail, "Updating the quota failed to preserve recorded usage"
	}
	if err := post(`{"adapter":"fixture","reset":true}`); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	after, err := database.GetAdapterCap(adapter)
	if err != nil || after == nil || after.DataQuotaMB != 12 || after.DataUsedMB != 0 {
		return testrunner.StatusFail, "Resetting usage failed to preserve the configured quota"
	}
	sched := engine.NewScheduler([]nics.Nic{{Name: adapter, Index: 1, IP: net.ParseIP("192.0.2.1"), Weight: 1}})
	sched.SetLimit(adapter, 8)
	sched.SetQuotaLimits(map[string]float64{adapter: 0.01})
	if sched.Snapshot()[0].LimitMbps != 0.01 || sched.Limits()[adapter] != 8 {
		return testrunner.StatusFail, "Quota throttle did not preserve the configured manual cap"
	}
	sched.SetQuotaLimits(nil)
	if sched.Snapshot()[0].LimitMbps != 8 {
		return testrunner.StatusFail, "Clearing the quota throttle failed to restore the manual cap"
	}
	return testrunner.StatusPass, "Verified quota edits, usage reset, and restoration of the manual speed cap"
}

func checkRuleEditor() (testrunner.TestStatus, string) {
	database, err := db.Open(":memory:")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer database.Close()
	policy, err := rules.NewEngine(database)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	svc := NetPilotService{database: database, ruleEngine: policy, ringLog: logger.NewRingLogger(8), modeCtrl: mode.NewController(capability.Report{})}
	rule := db.RuleRecord{ID: "rw", AppName: "test-client", AppPath: "/path with spaces/client", PreferredInterface: "net-merge", Priority: 0, DataQuotaMB: 10}
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		svc.ServeHTTP(w, req)
		return w
	}
	if res := call(http.MethodPost, "/api/rules", rule); res.Code != http.StatusOK {
		return testrunner.StatusFail, "Create policy: " + res.Body.String()
	}
	if err := database.AddRuleUsage(rule.ID, 2); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	rule.CustomDNS = "1.1.1.1"
	rule.AppPath = "/other path/client"
	if res := call(http.MethodPost, "/api/rules", rule); res.Code != http.StatusOK {
		return testrunner.StatusFail, "Edit policy: " + res.Body.String()
	}
	res := call(http.MethodGet, "/api/rules", nil)
	var rows []db.RuleRecord
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &rows) != nil || len(rows) != 1 ||
		rows[0].AppPath != rule.AppPath || rows[0].CustomDNS != rule.CustomDNS || rows[0].DataUsedMB != 2 ||
		rows[0].Priority != 0 || rows[0].Enabled {
		return testrunner.StatusFail, "Edited policy lost its existing ID, usage, priority, or paused state"
	}
	rule.Enabled = true
	if res := call(http.MethodPost, "/api/rules", rule); res.Code != http.StatusOK ||
		svc.Resolve("", rule.AppPath, "192.0.2.1").RuleID != rule.ID {
		return testrunner.StatusFail, "Enabled policy did not reach the routing engine"
	}
	if res := call(http.MethodDelete, "/api/rules?id=rw", nil); res.Code != http.StatusOK ||
		svc.Resolve("", rule.AppPath, "192.0.2.1").RuleID != "" {
		return testrunner.StatusFail, "Deleted policy remained active"
	}
	return testrunner.StatusPass, "Created, edited, enabled and deleted the same rule over the app API, with priority, paused state and quota usage verified"
}

func checkModeAndProfiles() (testrunner.TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-check-profile-")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	ctrl := netmerge.NewController(dir)
	defer ctrl.Close()
	store, err := profiles.Open(dir)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	svc := NetPilotService{netmergeCtrl: ctrl, profileStore: store}
	cfg := ctrl.GetConfig()
	cfg.RoutingMode, cfg.PrimaryAdapter, cfg.SecondaryAdapter = "backup", "fixture-a", "fixture-b"
	cfg.Scheduling, cfg.ProbeTargetsV4 = "low-latency", []string{"192.0.2.1:443"}
	profile, err := svc.SaveProfile(profiles.Profile{Name: "Diagnostic", NetMerge: cfg})
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	if err := ctrl.StartSocks5("127.0.0.1:0"); err != nil {
		return testrunner.StatusBlocked, "SOCKS listener: " + err.Error()
	}
	initial, _ := ctrl.GetStatus()
	client, err := net.DialTimeout("tcp4", initial.SocksAddress, time.Second)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if err := svc.ApplyProfile(context.Background(), profile.ID); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	after, _ := ctrl.GetStatus()
	if !after.SocksRunning || after.SocksAddress != initial.SocksAddress || after.RoutingMode != "backup" ||
		after.Config.PrimaryAdapter != cfg.PrimaryAdapter || after.Config.Scheduling != "low-latency" ||
		len(after.Config.ProbeTargetsV4) != 1 {
		return testrunner.StatusFail, "Applying Backup profile did not preserve the listener or save the selected routing and probes"
	}
	reply := make([]byte, 2)
	if _, err = client.Write([]byte{5, 1, 0}); err != nil {
		return testrunner.StatusFail, "Switch closed an established SOCKS client: " + err.Error()
	}
	if _, err = io.ReadFull(client, reply); err != nil || !bytes.Equal(reply, []byte{5, 0}) {
		return testrunner.StatusFail, "SOCKS client could not complete a handshake after mode changed"
	}

	state.Write(dir, state.Status{PID: os.Getpid(), Updated: time.Now(), Since: time.Now(), RoutingMode: "speed"})
	separate, _ := ctrl.GetStatus()
	state.Clear(dir)
	reopened, err := profiles.Open(dir)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	if _, ok := reopened.Get(profile.ID); !ok || svc.DeleteProfile(profile.ID) != nil {
		return testrunner.StatusFail, "The saved profile did not persist or could not be deleted"
	}
	if !separate.RestartForMode || !separate.ModeChangePending || separate.RoutingMode != "speed" || separate.Config.RoutingMode != "backup" {
		return testrunner.StatusFail, "Saved and running routing modes did not expose the restart requirement"
	}
	return testrunner.StatusPass, "A saved Backup profile kept a SOCKS client connected. A separate Speed process retained its running mode and showed the restart requirement"
}

func checkMetricsHistory() (testrunner.TestStatus, string) {
	database, err := db.Open(":memory:")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer database.Close()
	svc := NetPilotService{database: database, ringLog: logger.NewRingLogger(8)}
	day, month := time.Now().Format("2006-01-02"), time.Now().Format("2006-01")
	if err := database.RecordDailyUsage(day, "test", "", 2<<20, 1<<20); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	if err := database.RecordMonthlyUsage(month, "test", "", 2<<20, 1<<20); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	for _, period := range []string{"today", "week", "month", "all"} {
		if data := svc.GetUsageHistory(period); data.TotalMB != 3 || len(data.Records) != 1 || data.Records[0].BytesIn != 2<<20 {
			return testrunner.StatusFail, fmt.Sprintf("Usage history for %s returned incorrect totals", period)
		}
	}
	csv, err := svc.ExportMonthlyCSV(month)
	if err != nil || !strings.Contains(csv, month) || !strings.Contains(csv, "test") {
		return testrunner.StatusFail, "Monthly CSV omitted the recorded usage"
	}
	if _, err := svc.ResetDataUsage("daily", ""); err != nil ||
		svc.GetUsageHistory("today").TotalMB != 0 || svc.GetUsageHistory("month").TotalMB != 3 {
		return testrunner.StatusFail, "Resetting daily usage also changed monthly history or failed to clear today's data"
	}
	return testrunner.StatusPass, "Verified raw bytes and displayed totals across all ranges, monthly CSV, and selective history reset using an isolated database"
}

func checkNotificationPreferences() (testrunner.TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-check-notify-")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "notifications.json")
	var svc NetPilotService
	svc.initNotifications(path)
	if err := svc.setNotifications(notificationPreferences{Quotas: true}); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	svc.initNotifications(path)
	_ = svc.publishNotification("links", "Muted", "")
	options := svc.getNotifications()
	if options.Latest != nil || !options.Preferences.Quotas || options.Preferences.Desktop {
		return testrunner.StatusFail, "Persisted choices did not mute link events and desktop banners"
	}
	_ = svc.publishNotification("quotas", "Quota reached", "Fixture adapter")
	latest := svc.getNotifications().Latest
	if latest == nil || latest.Title != "Quota reached" {
		return testrunner.StatusFail, "An enabled event was missing from the in-app feed"
	}
	if result := svc.SendTestNotification(); result.Desktop != "off" || svc.getNotifications().Latest.Title != "NetPilot test notification" {
		return testrunner.StatusFail, "In-app test notification was not delivered"
	}
	return testrunner.StatusPass, "Verified persisted preferences, muted categories, and in-app quota and test notifications with desktop banners turned off"
}

func checkNativeNotificationFacility() (testrunner.TestStatus, string) {
	if !notify.Available() {
		return testrunner.StatusBlocked, "The host desktop notification service is unavailable"
	}
	return testrunner.StatusPass, "Desktop notification backend detected. Send a test in Settings to confirm that system permissions allow a visible banner."
}
