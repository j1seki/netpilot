package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
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
	if maybeRunNetMergeSubcommand(os.Args) {
		return
	}

	headless := flag.Bool("headless", false, "run headless without desktop window")
	runScenarios := flag.Bool("run-scenarios", false, "execute all simulation scenarios and exit")
	runSuite := flag.String("run-suite", "", "execute platform test suite (common, linux, windows, macos, scenarios) and exit")
	reqMode := flag.String("mode", "", "initial execution mode (dry-run or live); use -run-scenarios for simulations")
	dbPath := flag.String("db", "", "path to SQLite database (default: ~/.config/netpilot/netpilot.db)")
	showConsole := flag.Bool("console", false, "Show Windows console/terminal window (hidden by default)")
	startHidden := flag.Bool("start-hidden", false, "start with the main window hidden (tray only)")
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
		target := strings.ReplaceAll(strings.ToUpper(*reqMode), "-", "_")
		if _, err := modeCtrl.SetMode(mode.ExecutionMode(target), false); err != nil {
			fmt.Fprintf(os.Stderr, "invalid -mode %q: use dry-run or live (use -run-scenarios for simulations)\n", *reqMode)
			os.Exit(2)
		}
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

	scenarioEng := scenarios.NewEngine(ringLog, database)
	testRunner := testrunner.NewRunner(detector, database, ringLog)

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

	diagGen := diagnostics.NewGenerator(detector, modeCtrl, ringLog, database)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = analyticsCol.SampleHostNICs()
			}
		}
	}()
	go service.GetInterfaces()
	service.StartMonitors(ctx)

	// The desktop primarily uses the Wails route. Keep the loopback API for
	// headless clients and webviews that can't use that route for streaming.
	server := &http.Server{
		Addr:              localAPIAddress,
		Handler:           newLocalAPIHandler(service),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	defer server.Close()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ringLog.Log(logger.LogEntry{
				Level:     logger.LevelError,
				Component: "LocalAPI",
				Message:   fmt.Sprintf("Loopback API listener failed: %v", err),
			})
		}
	}()

	stopForwarding := func() {
		cancel()
		_ = server.Close()
		if err := service.stopForwarding(); err != nil {
			fmt.Fprintf(os.Stderr, "stop NetPilot forwarding on exit: %v\n", err)
		}
	}
	if *headless {
		signals, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Printf("%s v%s daemon running in headless mode (%s)...\n", branding.AppName, branding.AppVersion, modeCtrl.Current())
		<-signals.Done()
		stopForwarding()
		return
	}

	// 6. Launch Wails 3 Desktop Application with Cross-Platform System Tray & Close-to-Tray Hook
	// The updater belongs to the desktop app: CLI test runs and the headless
	// daemon must not check GitHub or send notifications.
	go service.updateLoop(ctx)
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
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	app.OnShutdown(stopForwarding)
	service.SetUpdateQuitFunc(app.Quit)

	mainWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     branding.WindowTitle,
		Width:     1320,
		Height:    860,
		MinWidth:  760,
		MinHeight: 540,
		Hidden:    *startHidden && isLinuxTrayHostAvailable(),
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

	// Initialize System Tray (combining NetPilot controls + all original net-merge tray options)
	sysTray := app.SystemTray.New()
	sysTray.SetIcon(icon.PNG(netmergeCtrl.IsActive()))
	sysTray.SetLabel("NetPilot")
	sysTray.SetTooltip("NetPilot — Multi-Adapter Network Control Center")

	trayMenu := app.NewMenu()

	// 1. Open NetPilot Dashboard
	openItem := trayMenu.Add("Open NetPilot")
	openItem.OnClick(func(_ *application.Context) {
		showMainWindow()
	})
	trayMenu.AddSeparator()

	// 2. Original net-merge Status & Total Speed Header
	statusItem := trayMenu.Add("Paused · System Default")
	statusItem.SetEnabled(false)

	// 3. Original net-merge Live Adapter Rows (up to 6 slots) with Speed Limit Submenus
	const maxTrayAdapters = 6
	trayAdapterLimits := []float64{0, 1, 2, 5, 10, 20, 50, 100}
	type trayAdapterSlot struct {
		sub    *application.MenuItem
		capOpt []*application.MenuItem
		name   atomic.Pointer[string]
	}
	adapterSlots := make([]trayAdapterSlot, maxTrayAdapters)
	for i := 0; i < maxTrayAdapters; i++ {
		slotIdx := i
		lbl := fmt.Sprintf("Adapter %d", i+1)
		subMenu := trayMenu.AddSubmenu(lbl)
		subItem := trayMenu.FindByLabel(lbl)
		headerItem := subMenu.Add("Speed limit")
		headerItem.SetEnabled(false)
		subMenu.AddSeparator()

		opts := make([]*application.MenuItem, len(trayAdapterLimits))
		for j, mbps := range trayAdapterLimits {
			capVal := mbps
			lbl := "Unlimited"
			if capVal > 0 {
				lbl = fmt.Sprintf("%.0f Mbit/s", capVal)
			}
			opt := subMenu.AddRadio(lbl, capVal == 0)
			opt.OnClick(func(_ *application.Context) {
				name := adapterSlots[slotIdx].name.Load()
				if name != nil {
					if err := service.SetNetMergeLimit(*name, capVal); err != nil {
						return
					}
					for k, o := range adapterSlots[slotIdx].capOpt {
						o.SetChecked(trayAdapterLimits[k] == capVal)
					}
					trayMenu.Update()
				}
			})
			opts[j] = opt
		}
		if subItem != nil {
			subItem.SetHidden(true)
		}
		adapterSlots[i].sub = subItem
		adapterSlots[i].capOpt = opts
	}

	trayMenu.AddSeparator()

	// 4. Start / Stop Merging (original net-merge toggle)
	toggleItem := trayMenu.Add("Start Net-Merge")
	toggleItem.OnClick(func(_ *application.Context) {
		want := !netmergeCtrl.IsActive()
		_ = service.SetNetMergeMode(want)
		active := netmergeCtrl.IsActive()
		if active {
			toggleItem.SetLabel("Stop Net-Merge")
		} else {
			toggleItem.SetLabel("Start Net-Merge")
		}
		sysTray.SetIcon(icon.PNG(active))
		trayMenu.Update()
	})

	trayMenu.AddSeparator()

	// 5. NetPilot Routing Modes: Speed vs Backup
	initCfg := netmergeCtrl.GetConfig()
	speedModeItem := trayMenu.AddRadio("Mode: Speed (Multi-Link Aggregation)", initCfg.RoutingMode != "backup")
	backupModeItem := trayMenu.AddRadio("Mode: Backup (Primary -> Failover)", initCfg.RoutingMode == "backup")

	speedModeItem.OnClick(func(_ *application.Context) {
		_ = service.SetNetMergeRoutingMode("speed", "", "")
		speedModeItem.SetChecked(true)
		backupModeItem.SetChecked(false)
		trayMenu.Update()
	})
	backupModeItem.OnClick(func(_ *application.Context) {
		if err := service.SetNetMergeRoutingMode("backup", "", ""); err == nil {
			backupModeItem.SetChecked(true)
			speedModeItem.SetChecked(false)
		}
		trayMenu.Update()
	})

	trayMenu.AddSeparator()

	// 6. Original net-merge checkboxes: Use all adapters (IPv6 auto), Pause on single connection, Start at sign-in
	useAllItem := trayMenu.AddCheckbox("Use all adapters (IPv6 only if every adapter has it)", initCfg.IPv6Mode != "on")
	useAllItem.OnClick(func(_ *application.Context) {
		cur := netmergeCtrl.GetConfig()
		nextMode := "auto"
		if cur.IPv6Mode != "on" {
			nextMode = "on"
		}
		_ = netmergeCtrl.SetIPv6Mode(nextMode)
		useAllItem.SetChecked(nextMode != "on")
		trayMenu.Update()
	})

	pauseSingleItem := trayMenu.AddCheckbox("Pause when there is only one connection", initCfg.AutoPause)
	pauseSingleItem.OnClick(func(_ *application.Context) {
		cur := netmergeCtrl.GetConfig()
		next := !cur.AutoPause
		_ = netmergeCtrl.SetAutoPause(next)
		pauseSingleItem.SetChecked(next)
		trayMenu.Update()
	})

	autostartItem := trayMenu.AddCheckbox("Start at sign-in", initCfg.Autostart)
	autostartItem.OnClick(func(_ *application.Context) {
		cur := netmergeCtrl.GetConfig()
		next := !cur.Autostart
		if err := service.SetNetMergeAutostart(next); err == nil {
			autostartItem.SetChecked(next)
		} else {
			autostartItem.SetChecked(cur.Autostart)
		}
		trayMenu.Update()
	})

	trayMenu.AddSeparator()

	// 7. Version info & Quit
	verItem := trayMenu.Add(fmt.Sprintf("NetPilot v%s (net-merge %s)", branding.AppVersion, netmerge.EmbeddedVersion()))
	verItem.SetEnabled(false)

	quitItem := trayMenu.Add("Quit NetPilot")
	quitItem.OnClick(func(_ *application.Context) {
		app.Quit()
	})

	sysTray.SetMenu(trayMenu)
	sysTray.OnClick(func() {
		showMainWindow()
	})

	// Background goroutine to keep tray icon, adapter speeds, and menu options synchronized
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			st, err := netmergeCtrl.GetStatus()
			if err != nil {
				continue
			}
			active := st.IsRunning
			cfg := st.Config
			liveMode := modeCtrl.Current() == mode.ModeLive
			engineLabel := "Selected: " + st.Binary.Version
			if active {
				engineLabel = "Running: " + st.RunningVersion
				if st.RunningVersion == "" {
					engineLabel = "Running: version not reported"
				}
			} else if st.Binary.Version == "" {
				engineLabel = "Selected engine unavailable"
			}
			verItem.SetLabel(engineLabel)

			sysTray.SetIcon(icon.PNG(active))
			toggleItem.SetEnabled(liveMode)
			if active {
				toggleItem.SetLabel("Stop Net-Merge")
			} else {
				toggleItem.SetLabel("Start Net-Merge")
			}
			speedModeItem.SetChecked(cfg.RoutingMode != "backup")
			backupModeItem.SetChecked(cfg.RoutingMode == "backup")
			backupModeItem.SetEnabled(cfg.EngineSource == "builtin")
			useAllItem.SetChecked(cfg.IPv6Mode != "on")
			pauseSingleItem.SetChecked(cfg.AutoPause)
			autostartItem.SetChecked(st.Autostart)
			autostartItem.SetEnabled(liveMode)

			// Update live adapter rows and aggregate speed
			var totalDown, totalUp float64
			upCount := 0
			presentIdx := 0
			for _, nic := range st.NICs {
				if !nic.Present {
					continue
				}
				totalDown += nic.LinkIn
				totalUp += nic.LinkOut
				if nic.State == "up" {
					upCount++
				}
				if presentIdx < maxTrayAdapters {
					slot := &adapterSlots[presentIdx]
					slot.name.Store(&nic.Name)
					if slot.sub != nil {
						stateSym := "✓"
						if nic.State == "no internet" {
							stateSym = "✗"
						} else if nic.State == "checking" {
							stateSym = "…"
						}
						downMbps := nic.LinkIn * 8 / 1e6
						upMbps := nic.LinkOut * 8 / 1e6
						limSuffix := ""
						if nic.LimitMbps > 0 {
							limSuffix = fmt.Sprintf(" (≤%.0fM)", nic.LimitMbps)
						}
						slot.sub.SetLabel(fmt.Sprintf("%s %s: ↓%.1f ↑%.1f Mbps%s", stateSym, nic.Name, downMbps, upMbps, limSuffix))
						slot.sub.SetHidden(false)
						for k, opt := range slot.capOpt {
							opt.SetChecked(trayAdapterLimits[k] == nic.LimitMbps)
							opt.SetEnabled(liveMode)
						}
					}
					presentIdx++
				}
			}
			for i := presentIdx; i < maxTrayAdapters; i++ {
				if adapterSlots[i].sub != nil {
					adapterSlots[i].sub.SetHidden(true)
					adapterSlots[i].name.Store(nil)
				}
			}

			totalMbps := (totalDown + totalUp) * 8 / 1e6
			if active {
				modeTag := "Speed"
				if cfg.RoutingMode == "backup" {
					modeTag = "Backup"
				}
				statusItem.SetLabel(fmt.Sprintf("Merging %d adapters (%s) — %.1f Mbps", upCount, modeTag, totalMbps))
			} else {
				statusItem.SetLabel("Paused · System Default (Standby)")
			}
			trayMenu.Update()
		}
	}()

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application run error: %v\n", err)
		os.Exit(1)
	}
}
