package main

import (
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"netpilot/frontend"
	"netpilot/internal/analytics"
	"netpilot/internal/apps"
	"netpilot/internal/branding"
	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/diagnostics"
	"netpilot/internal/failover"
	"netpilot/internal/icon"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
	"netpilot/internal/netmerge"
	"netpilot/internal/rules"
	"netpilot/internal/scenarios"
	"netpilot/internal/state"
	"netpilot/internal/testrunner"
	"netpilot/internal/ttlmgr"
)

func main() {
	headless := flag.Bool("headless", false, "run headless without desktop window")
	runScenarios := flag.Bool("run-scenarios", false, "execute all simulation scenarios and exit")
	runSuite := flag.String("run-suite", "", "execute platform test suite (common, linux, windows, macos, scenarios) and exit")
	reqMode := flag.String("mode", "", "initial execution mode (simulation, dry-run, live)")
	dbPath := flag.String("db", "", "path to SQLite database (default: ~/.config/netpilot/netpilot.db)")
	showConsole := flag.Bool("console", false, "Show Windows console/terminal window (hidden by default)")
	flag.Parse()

	if *showConsole {
		_ = setConsoleVisible(true)
	}

	// 1. Initialize Structured Logger
	ringLog := logger.NewRingLogger(10000)
	ringLog.Log(logger.LogEntry{
		Level:     logger.LevelInfo,
		Component: "Boot",
		Message:   fmt.Sprintf("Starting %s v%s (%s)", branding.AppName, branding.AppVersion, branding.AppID),
	})

	// 2. Capability Detection
	detector := capability.NewDetector()
	capReport := detector.Detect()
	ringLog.Log(logger.LogEntry{
		Level:     logger.LevelInfo,
		Component: "CapabilityDetector",
		Message:   fmt.Sprintf("Detected %s on %s/%s (VM: %v, Type: %s)", capReport.Environment.RuntimeOS, capReport.Environment.RuntimeOS, capReport.Environment.RuntimeArch, capReport.Environment.IsVM, capReport.Environment.VMType),
	})

	// 3. Mode Controller
	modeCtrl := mode.NewController(capReport)
	if *reqMode != "" {
		_, _ = modeCtrl.SetMode(mode.ExecutionMode(stringsToUpper(*reqMode)), false)
	}

	// 4. SQLite Database
	actualDBPath := *dbPath
	if actualDBPath == "" {
		home, _ := os.UserHomeDir()
		actualDBPath = filepath.Join(home, ".config", "netpilot", "netpilot.db")
	}
	database, err := db.Open(actualDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize SQLite database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 5. Core Subsystems
	ruleEng, err := rules.NewEngine(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize rules engine: %v\n", err)
		os.Exit(1)
	}

	failoverSM := failover.NewStateMachine(failover.Config{
		FailThreshold:    3,
		RecoverThreshold: 2,
	}, func(iface string, oldState, newState failover.LinkState) {
		ringLog.Log(logger.LogEntry{
			Level:     logger.LevelWarn,
			Component: "FailoverEngine",
			Message:   fmt.Sprintf("Interface %s transitioned %s -> %s", iface, oldState, newState),
		})
	})

	appMgr := apps.NewManager()
	analyticsCol := analytics.NewCollector(database)

	// Wire dynamic lowest-latency interface selection for rules set to "FASTEST"
	ruleEng.SetFastestNICProvider(failoverSM.GetFastestInterface)

	// Seed baseline monthly data & adapter caps if not yet initialized
	currentYM := time.Now().Format("2006-01")
	if hist, _ := database.GetMonthlyHistory(currentYM); len(hist) == 0 {
		_ = database.RecordMonthlyUsage("2026-10", "eth0", "system-update", 650*1024*1024, 120*1024*1024)
		_ = database.RecordMonthlyUsage("2026-10", "tailscale0", "orca-ide", 240*1024*1024, 75*1024*1024)
		_ = database.RecordMonthlyUsage(currentYM, "eth0", "netpilot", 38*1024*1024, 12*1024*1024)
		_ = database.RecordMonthlyUsage(currentYM, "tailscale0", "tailscaled", 45*1024*1024, 28*1024*1024)
	}
	if caps, _ := database.GetAdapterCaps(); len(caps) == 0 {
		_ = database.SaveAdapterCap(db.AdapterCapRecord{
			InterfaceName: "eth0",
			RxLimitMbps:   100.0,
			TxLimitMbps:   50.0,
			DataQuotaMB:   50000.0,
			DataUsedMB:    184.2,
			Enabled:       true,
		})
	}

	// Sample real host network counters in background
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_ = analyticsCol.SampleHostNICs()
		}
	}()

	scenarioEng := scenarios.NewEngine(ringLog, database)
	diagGen := diagnostics.NewGenerator(detector, modeCtrl, ringLog, database)
	testRunner := testrunner.NewRunner(detector, database, ringLog)
	netmergeCtrl := netmerge.NewController("")
	ttlStore := ttlmgr.NewStore(filepath.Join(state.DefaultDir(), "ttl"))
	ttlBackend := ttlmgr.NewBackend(ttlmgr.NewExecRunner())
	ttlManager := ttlmgr.NewManager(ttlStore, ttlBackend)

	service := NewNetPilotService(
		database,
		ringLog,
		detector,
		modeCtrl,
		ruleEng,
		failoverSM,
		appMgr,
		analyticsCol,
		scenarioEng,
		diagGen,
		testRunner,
		netmergeCtrl,
		ttlManager,
	)

	// Trigger initial WAN IP discovery for active interfaces
	go func() {
		_ = service.GetInterfaces()
	}()

	// Start local loopback HTTP API listener for live inspection & testing
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			service.ServeHTTP(w, r)
		})
		_ = http.ListenAndServe("127.0.0.1:4545", mux)
	}()

	// CLI execution paths
	if *runScenarios {
		fmt.Printf("=== Running All Deterministic Simulation Scenarios for %s ===\n", branding.AppName)
		results := scenarioEng.RunAll()
		passed, failed := 0, 0
		for _, r := range results {
			statusSymbol := "✓"
			if r.Status != scenarios.StatusPass {
				statusSymbol = "✗"
				failed++
			} else {
				passed++
			}
			fmt.Printf("[%s] %s %-45s (%.2fms)\n", r.Category, statusSymbol, r.Name, r.DurationMs)
		}
		fmt.Printf("\nTotal: %d | Passed: %d | Failed: %d\n", len(results), passed, failed)
		if failed > 0 {
			os.Exit(1)
		}
		return
	}

	if *runSuite != "" {
		fmt.Printf("=== Running Platform Test Suite: %s ===\n", *runSuite)
		rep := testRunner.RunSuite(testrunner.PlatformSuite(*runSuite))
		fmt.Printf("Suite: %s | Target: %s | Total: %d | Passed: %d | Failed: %d | Skipped: %d | Blocked: %d (%.2fms)\n",
			rep.Suite, rep.TargetOS, rep.Total, rep.Passed, rep.Failed, rep.Skipped, rep.Blocked, rep.DurationMs)
		for _, c := range rep.Cases {
			fmt.Printf("  - [%s] %-8s %-40s (%.2fms) %s\n", c.Classification, c.Status, c.Name, c.DurationMs, c.Reason)
		}
		if rep.Failed > 0 {
			os.Exit(1)
		}
		return
	}

	if *headless {
		fmt.Printf("%s v%s daemon running in headless mode (%s)...\n", branding.AppName, branding.AppVersion, modeCtrl.Current())
		select {}
	}

	// 6. Launch Wails 3 Desktop Application with Cross-Platform System Tray & Close-to-Tray Hook
	prepareLinuxTrayEnvironment()

	distFS, err := fs.Sub(frontend.DistFS, "dist")
	var assetHandler http.Handler
	if err == nil {
		assetHandler = application.BundledAssetFileServer(distFS)
	}

	app := application.New(application.Options{
		Name:        branding.AppName,
		Description: branding.AppDescription,
		Icon:        icon.PNG(true),
		Services: []application.Service{
			application.NewServiceWithOptions(service, application.ServiceOptions{
				Route: "/api",
			}),
		},
		Assets: application.AssetOptions{
			Handler: assetHandler,
		},
	})

	mainWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     branding.WindowTitle,
		Width:     1320,
		Height:    860,
		MinWidth:  1024,
		MinHeight: 650,
	})

	showMainWindow := func() {
		mainWin.Show()
		mainWin.Restore()
		mainWin.Focus()
	}

	// Register window close hook: check user preference ("minimize" to tray by default vs "close" app)
	mainWin.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		closeAction := service.GetCloseAction()
		if closeAction == "close" {
			return // allow normal close & application exit
		}
		// Minimize to system tray (or task bar if Linux DE has no StatusNotifierWatcher at all)
		event.Cancel()
		if isLinuxTrayHostAvailable() {
			mainWin.Hide()
		} else {
			mainWin.Minimise()
		}
	})

	// Initialize System Tray
	sysTray := app.SystemTray.New()
	sysTray.SetIcon(icon.PNG(netmergeCtrl.IsActive()))
	sysTray.SetLabel("NetPilot")
	sysTray.SetTooltip("NetPilot — Multi-Adapter Network Control Center")

	trayMenu := app.NewMenu()
	openItem := trayMenu.Add("Open NetPilot")
	openItem.OnClick(func(_ *application.Context) {
		showMainWindow()
	})

	trayMenu.AddSeparator()
	statusItem := trayMenu.Add("Adapter: System Default (Standby)")
	statusItem.SetEnabled(false)

	toggleItem := trayMenu.AddCheckbox("Enable Net-Merge Adapter", netmergeCtrl.IsActive())
	toggleItem.OnClick(func(_ *application.Context) {
		want := !netmergeCtrl.IsActive()
		_ = service.SetNetMergeMode(want)
		toggleItem.SetChecked(netmergeCtrl.IsActive())
		sysTray.SetIcon(icon.PNG(netmergeCtrl.IsActive()))
	})

	trayMenu.AddSeparator()
	speedModeItem := trayMenu.AddRadio("Mode: Speed (Multi-Link Aggregation)", netmergeCtrl.GetConfig().RoutingMode != "backup")
	backupModeItem := trayMenu.AddRadio("Mode: Backup (Primary -> Failover)", netmergeCtrl.GetConfig().RoutingMode == "backup")

	speedModeItem.OnClick(func(_ *application.Context) {
		_ = service.SetNetMergeRoutingMode("speed", "", "")
		speedModeItem.SetChecked(true)
		backupModeItem.SetChecked(false)
	})
	backupModeItem.OnClick(func(_ *application.Context) {
		_ = service.SetNetMergeRoutingMode("backup", "", "")
		backupModeItem.SetChecked(true)
		speedModeItem.SetChecked(false)
	})

	trayMenu.AddSeparator()
	quitItem := trayMenu.Add("Quit NetPilot")
	quitItem.OnClick(func(_ *application.Context) {
		_ = service.SetNetMergeMode(false)
		app.Quit()
	})

	sysTray.SetMenu(trayMenu)
	sysTray.OnClick(func() {
		showMainWindow()
	})

	// Background goroutine to keep tray icon & status synchronized with Net-Merge state
	go func() {
		lastActive := !netmergeCtrl.IsActive()
		lastMode := ""
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			active := netmergeCtrl.IsActive()
			cfg := netmergeCtrl.GetConfig()
			if active != lastActive || cfg.RoutingMode != lastMode {
				lastActive = active
				lastMode = cfg.RoutingMode
				sysTray.SetIcon(icon.PNG(active))
				toggleItem.SetChecked(active)
				speedModeItem.SetChecked(cfg.RoutingMode != "backup")
				backupModeItem.SetChecked(cfg.RoutingMode == "backup")
				if active {
					if cfg.RoutingMode == "backup" {
						statusItem.SetLabel("Active: net-merge (Backup Mode)")
					} else {
						statusItem.SetLabel("Active: net-merge (Speed Mode)")
					}
				} else {
					statusItem.SetLabel("Adapter: System Default (Standby)")
				}
			}
		}
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application run error: %v\n", err)
		os.Exit(1)
	}
}

func stringsToUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}
