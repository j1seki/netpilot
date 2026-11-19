package netmerge

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"netpilot/internal/engine"
	"netpilot/internal/execx"
	"netpilot/internal/logx"
	"netpilot/internal/netmerge/embedded"
	"netpilot/internal/nics"
	"netpilot/internal/platform"
	"netpilot/internal/state"
	"netpilot/internal/tun"
)

// Config represents all controllable parameters of the net-merge engine and NetPilot behavior.
type Config struct {
	AutoPause        bool               `json:"autoPause"`
	AutoWeight       bool               `json:"autoWeight"`
	IPv6Mode         string             `json:"ipv6Mode"` // "auto", "on", "off"
	MTU              int                `json:"mtu"`      // 1280 - 65535
	IncludeNICs      string             `json:"includeNics"`
	RescanSec        int                `json:"rescanSec"`
	SocksAddress     string             `json:"socksAddress"` // e.g. "127.0.0.1:1080"
	Weights          map[string]float64 `json:"weights"`
	ShowConsole      bool               `json:"showConsole"`
	Autostart        bool               `json:"autostart"`
	EngineSource     string             `json:"engineSource"` // "builtin" (default) or "external"
	BinaryPath       string             `json:"binaryPath,omitempty"`
	RoutingMode      string             `json:"routingMode"`      // "speed" (default) or "backup"
	PrimaryAdapter   string             `json:"primaryAdapter"`   // primary NIC name in backup mode
	SecondaryAdapter string             `json:"secondaryAdapter"` // secondary/failover NIC name in backup mode
	CloseAction      string             `json:"closeAction"`      // "minimize" (default) or "close"
	Scheduling       string             `json:"scheduling"`       // "balanced" (default) or "low-latency"
	ProbeTargetsV4   []string           `json:"probeTargetsV4,omitempty"`
	ProbeTargetsV6   []string           `json:"probeTargetsV6,omitempty"`
}

// StatusInfo represents the runtime state of net-merge.
type StatusInfo struct {
	IsRunning      bool               `json:"isRunning"`
	PID            int                `json:"pid"`
	Mode           string             `json:"mode"`
	RoutingMode    string             `json:"routingMode"`    // "speed" or "backup"
	DefaultAdapter string             `json:"defaultAdapter"` // "net-merge" when active, "System Default" when inactive
	ActiveAdapter  string             `json:"activeAdapter"`  // currently active primary/failover adapter or "net-merge"
	V6State        string             `json:"v6State"`
	UptimeSec      int64              `json:"uptimeSec"`
	Autostart      bool               `json:"autostart"`
	SocksRunning   bool               `json:"socksRunning"`
	SocksAddress   string             `json:"socksAddress"`
	Config         Config             `json:"config"`
	NICs           []engine.SlotStats `json:"nics"`
	Limits         map[string]float64 `json:"limits"`
	Binary         BinaryInfo         `json:"binary"`
	RunningVersion string             `json:"runningVersion,omitempty"`
	RunningSource  string             `json:"runningSource,omitempty"`
	RestartForMode bool               `json:"restartForMode"`
	SystemAppRules bool               `json:"systemAppRules"`
}

// Controller coordinates all net-merge operations directly inside NetPilot.
type Controller struct {
	mu           sync.RWMutex
	installMu    sync.Mutex
	stateDir     string
	configFile   string
	config       Config
	socksLn      net.Listener
	socksCancel  context.CancelFunc
	socksRunning bool
	socksAddr    string

	// In-process scheduler and balancer
	sched         *engine.Scheduler
	balancer      *engine.Balancer
	tunDev        *tun.Tun
	tunOn         bool
	tunSince      time.Time
	stopCh        chan struct{}
	cachedAuto    bool
	lastAutoAt    time.Time
	launchedPID   int
	launched      BinaryInfo
	launchedMode  string
	launchedSince time.Time
}

// NewController creates and initializes the Net-Merge controller with a live scheduler.
func NewController(stateDir string) *Controller {
	configDir := stateDir
	if stateDir == "" {
		stateDir = state.DefaultDir()
		if runtime.GOOS == "darwin" {
			configDir = ""
			if base, err := os.UserConfigDir(); err == nil {
				configDir = filepath.Join(base, "NetPilot")
			}
		} else {
			configDir = stateDir
		}
	}
	_ = state.Prepare(stateDir)

	return newController(stateDir, configDir)
}

func newController(stateDir, configDir string) *Controller {
	cfgPath := ""
	if configDir != "" {
		cfgPath = filepath.Join(configDir, "netpilot-netmerge-config.json")
	}
	cfg := Config{
		AutoPause:    true,
		AutoWeight:   true,
		IPv6Mode:     "auto",
		MTU:          tun.DefaultMTU,
		IncludeNICs:  "",
		RescanSec:    3,
		SocksAddress: "127.0.0.1:1080",
		Weights:      make(map[string]float64),
		ShowConsole:  false,
		EngineSource: "builtin",
		RoutingMode:  "speed",
		CloseAction:  "minimize",
		Scheduling:   "balanced",
	}

	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	cfg = normalizeConfig(cfg)

	c := &Controller{
		stateDir:   stateDir,
		configFile: cfgPath,
		config:     cfg,
		socksAddr:  cfg.SocksAddress,
		stopCh:     make(chan struct{}),
	}

	c.initScheduler()
	return c
}

func normalizeConfig(cfg Config) Config {
	if cfg.IPv6Mode == "" {
		cfg.IPv6Mode = "auto"
	}
	if cfg.MTU < 1280 || cfg.MTU > 65535 {
		cfg.MTU = tun.DefaultMTU
	}
	if cfg.RescanSec <= 0 {
		cfg.RescanSec = 3
	}
	if cfg.SocksAddress == "" {
		cfg.SocksAddress = "127.0.0.1:1080"
	}
	if cfg.Weights == nil {
		cfg.Weights = make(map[string]float64)
	}
	if cfg.EngineSource != "external" && cfg.EngineSource != "bundled" && cfg.EngineSource != "release" {
		if strings.TrimSpace(cfg.BinaryPath) != "" && cfg.EngineSource == "" {
			cfg.EngineSource = "external"
		} else {
			cfg.EngineSource = "builtin"
		}
	}
	if cfg.RoutingMode != "backup" {
		cfg.RoutingMode = "speed"
	}
	if cfg.EngineSource != "builtin" {
		cfg.RoutingMode = "speed"
	}
	if cfg.CloseAction != "close" {
		cfg.CloseAction = "minimize"
	}
	if cfg.Scheduling != "low-latency" {
		cfg.Scheduling = "balanced"
	}
	return cfg
}

func (c *Controller) parseIncludes() []string {
	var inc []string
	for _, s := range strings.Split(c.config.IncludeNICs, ",") {
		if s = strings.TrimSpace(s); s != "" {
			inc = append(inc, s)
		}
	}
	return inc
}

func (c *Controller) discover() ([]nics.Nic, error) {
	found, _, err := nics.DiscoverNicsWithFallback(c.parseIncludes(), c.config.Weights)
	if err != nil {
		return nil, fmt.Errorf("discover network adapters: %w", err)
	}
	return found, nil
}

// updateSchedulerFromDiscovery applies only successful scans. In particular, an empty
// successful result is meaningful and removes vanished adapters; an error must not do so.
func updateSchedulerFromDiscovery(sched *engine.Scheduler, found []nics.Nic, discoveryErr error) error {
	if discoveryErr != nil {
		return discoveryErr
	}
	sched.Update(found)
	return nil
}

func (c *Controller) applySchedulerModeLocked() {
	if c.sched == nil {
		return
	}
	c.sched.SetAuto(c.config.AutoWeight && len(c.config.Weights) == 0)
	c.sched.SetLatencyAware(c.config.Scheduling == "low-latency")
	c.sched.SetProbeTargets(c.config.ProbeTargetsV4, c.config.ProbeTargetsV6)
	if c.config.RoutingMode == "backup" {
		c.sched.SetMode(engine.ModeBackup, c.config.PrimaryAdapter, c.config.SecondaryAdapter)
	} else {
		c.sched.SetMode(engine.ModeSpeed, c.config.PrimaryAdapter, c.config.SecondaryAdapter)
	}
}

func (c *Controller) initScheduler() {
	initial, err := c.discover()
	if err != nil {
		logx.Printf("warning: initial adapter discovery failed: %v\n", err)
		initial = nil
	}
	c.sched = engine.NewScheduler(initial)
	if c.config.PrimaryAdapter == "" && len(initial) > 0 {
		c.config.PrimaryAdapter = initial[0].Name
	}
	if c.config.SecondaryAdapter == "" && len(initial) > 1 {
		c.config.SecondaryAdapter = initial[1].Name
	}
	c.applySchedulerModeLocked()

	if limits, err := state.LoadLimits(c.stateDir); err == nil {
		c.sched.SetLimits(limits)
	}
	c.balancer = engine.NewBalancer(c.sched)
	if v6, ok := engine.ParseV6Mode(c.config.IPv6Mode); ok {
		c.balancer.SetV6Mode(v6)
	}

	go c.sched.RunProbes(c.stopCh)
	go func() {
		rescanTicker := time.NewTicker(3 * time.Second)
		tickTicker := time.NewTicker(1 * time.Second)
		defer rescanTicker.Stop()
		defer tickTicker.Stop()
		last := time.Now()
		for {
			select {
			case <-c.stopCh:
				return
			case <-rescanTicker.C:
				c.mu.RLock()
				found, err := c.discover()
				c.mu.RUnlock()
				if err := updateSchedulerFromDiscovery(c.sched, found, err); err != nil {
					logx.Printf("warning: adapter rescan failed: %v\n", err)
				}
			case now := <-tickTicker.C:
				c.sched.Tick(now.Sub(last))
				last = now
				c.mu.RLock()
				isOn := c.tunOn
				since := c.tunSince
				rm := c.config.RoutingMode
				if isOn {
					modeDesc := "system-wide (Speed Mode)"
					if rm == "backup" {
						modeDesc = "system-wide (Backup Failover)"
					}
					state.Write(c.stateDir, state.Status{
						Version: nativeVersionLabel(), EngineSource: "builtin", RoutingMode: rm,
						PID:     os.Getpid(),
						Mode:    modeDesc,
						V6:      c.balancer.V6State(),
						Since:   since,
						Updated: now,
						NICs:    c.sched.Snapshot(),
					})
				}
				c.mu.RUnlock()
			}
		}
	}()
}

func (c *Controller) checkAutostartCached() bool {
	if time.Since(c.lastAutoAt) > 30*time.Second {
		c.cachedAuto = platform.DesktopAutostartEnabled()
		c.lastAutoAt = time.Now()
	}
	return c.cachedAuto
}

// IsActive returns true if net-merge is currently active (either in-process or via status.json).
// When IsActive is false, NetPilot's default adapter is "System Default" and NetPilot does nothing
// (pure standby).
func (c *Controller) IsActive() bool {
	c.mu.RLock()
	inProc := c.tunOn
	stateDir := c.stateDir
	c.mu.RUnlock()
	if inProc {
		return true
	}
	_, ok := state.Read(stateDir)
	return ok
}

// IsForwarding reports whether the TUN or the in-app SOCKS proxy is active.
func (c *Controller) IsForwarding() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tunOn || c.socksRunning {
		return true
	}
	_, ok := state.Read(c.stateDir)
	return ok
}

// ProcessRates reports attributed traffic from the integrated TUN or SOCKS engine.
func (c *Controller) ProcessRates() map[int]float64 {
	c.mu.RLock()
	live := c.tunOn || c.socksRunning
	b := c.balancer
	c.mu.RUnlock()
	if live && b != nil {
		return b.ProcessRates(time.Now())
	}
	if st, ok := state.Read(c.stateDir); ok {
		return st.ProcessMbps
	}
	return nil
}

// DefaultAdapterName returns "net-merge" when net-merge is active, or "System Default" when inactive.
func (c *Controller) DefaultAdapterName() string {
	if c.IsActive() {
		return "net-merge"
	}
	return "System Default"
}

// GetCloseAction returns the configured window close action ("minimize" or "close").
func (c *Controller) GetCloseAction() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.config.CloseAction == "close" {
		return "close"
	}
	return "minimize"
}

// SetCloseAction updates the window close button behavior ("minimize" to tray or "close" app).
func (c *Controller) SetCloseAction(action string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.EqualFold(strings.TrimSpace(action), "close") {
		c.config.CloseAction = "close"
	} else {
		c.config.CloseAction = "minimize"
	}
	return c.saveConfigLocked()
}

// SetRoutingMode updates the routing mode ("speed" vs "backup") and primary/secondary adapters.
func (c *Controller) SetRoutingMode(mode, primary, secondary string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.config
	if strings.EqualFold(strings.TrimSpace(mode), "backup") {
		if c.config.EngineSource != "builtin" {
			return fmt.Errorf("Backup Failover requires the NetPilot engine; switch engines in Settings first")
		}
		c.config.RoutingMode = "backup"
	} else {
		c.config.RoutingMode = "speed"
	}
	if primary != "" {
		c.config.PrimaryAdapter = primary
	}
	if secondary != "" {
		c.config.SecondaryAdapter = secondary
	}
	if err := c.saveConfigLocked(); err != nil {
		c.config = previous
		return err
	}
	c.applySchedulerModeLocked()
	return nil
}

// SetScheduling switches between even load balancing and latency-aware scheduling.
func (c *Controller) SetScheduling(profile string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.EqualFold(strings.TrimSpace(profile), "low-latency") {
		c.config.Scheduling = "low-latency"
	} else {
		c.config.Scheduling = "balanced"
	}
	c.applySchedulerModeLocked()
	return c.saveConfigLocked()
}

// SetProbeTargets changes the reachability probe hosts. Empty lists restore the defaults.
func (c *Controller) SetProbeTargets(v4, v6 []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.ProbeTargetsV4 = append([]string(nil), v4...)
	c.config.ProbeTargetsV6 = append([]string(nil), v6...)
	c.applySchedulerModeLocked()
	return c.saveConfigLocked()
}

// SetPolicyResolver wires the per-application rule engine into the live balancer.
func (c *Controller) SetPolicyResolver(r engine.PolicyResolver) {
	c.mu.RLock()
	b := c.balancer
	c.mu.RUnlock()
	if b != nil {
		b.SetPolicyResolver(r)
	}
}

func resolveActiveAdapter(isRunning bool, cfg Config, snap []engine.SlotStats) string {
	if !isRunning {
		return "System Default"
	}
	if cfg.RoutingMode == "backup" {
		// Check if PrimaryAdapter is present and up
		for _, s := range snap {
			if s.Name == cfg.PrimaryAdapter && s.Present && s.State != "no internet" {
				return s.Name + " (Primary)"
			}
		}
		for _, s := range snap {
			if s.Name == cfg.SecondaryAdapter && s.Present && s.State != "no internet" {
				return s.Name + " (Backup Active)"
			}
		}
		for _, s := range snap {
			if s.Present && s.State != "no internet" {
				return s.Name + " (Failover)"
			}
		}
	}
	return "net-merge"
}

// GetStatus returns the current net-merge status and slot statistics without spawning processes on every poll.
func (c *Controller) GetStatus() (StatusInfo, error) {
	c.mu.Lock()
	auto := c.checkAutostartCached()
	c.config.Autostart = auto
	c.mu.Unlock()

	c.mu.RLock()
	defer c.mu.RUnlock()
	inProcOn, inProcSince, cfg := c.tunOn, c.tunSince, c.config

	snap := []engine.SlotStats{}
	if c.sched != nil {
		snap = c.sched.Snapshot()
	}

	binInfo := c.resolve(cfg.EngineSource, cfg.BinaryPath)

	info := StatusInfo{
		IsRunning:      inProcOn,
		RoutingMode:    cfg.RoutingMode,
		DefaultAdapter: "System Default",
		ActiveAdapter:  "System Default",
		Autostart:      auto,
		SocksRunning:   c.socksRunning,
		SocksAddress:   c.socksAddr,
		Config:         cfg,
		NICs:           snap,
		Limits:         make(map[string]float64),
		Binary:         binInfo,
	}

	if lims, err := state.LoadLimits(c.stateDir); err == nil {
		info.Limits = lims
	}

	if inProcOn {
		info.SystemAppRules = true
		info.RunningVersion = nativeVersionLabel()
		info.RunningSource = "builtin"
		info.PID = os.Getpid()
		if cfg.RoutingMode == "backup" {
			info.Mode = "system-wide (Backup Mode)"
		} else {
			info.Mode = "system-wide (Speed Mode)"
		}
		info.DefaultAdapter = "net-merge"
		info.ActiveAdapter = resolveActiveAdapter(true, cfg, snap)
		if c.balancer != nil {
			info.V6State = c.balancer.V6State()
		}
		info.UptimeSec = int64(time.Since(inProcSince).Seconds())
		return info, nil
	}

	// Check if an external net-merge daemon/tray wrote status.json
	if st, ok := state.Read(c.stateDir); ok {
		info.IsRunning = true
		info.PID = st.PID
		info.Mode = st.Mode
		info.RunningVersion = st.Version
		info.RunningSource = st.EngineSource
		info.RoutingMode = "speed"
		if st.RoutingMode == "backup" {
			info.RoutingMode = "backup"
		}
		if st.PID == c.launchedPID && (st.Since.Equal(c.launchedSince) || st.EngineSource == "builtin") {
			info.RunningVersion = c.launched.Version
			info.RunningSource = c.launched.EngineSource
			info.RoutingMode = c.launchedMode
		}
		info.RestartForMode = true
		info.V6State = st.V6
		info.UptimeSec = int64(time.Since(st.Since).Seconds())
		if len(st.NICs) > 0 {
			info.NICs = st.NICs
		}
		info.DefaultAdapter = "net-merge"
		if info.RoutingMode == "backup" {
			live := Config{RoutingMode: "backup", PrimaryAdapter: st.PrimaryAdapter, SecondaryAdapter: st.SecondaryAdapter}
			info.ActiveAdapter = resolveActiveAdapter(true, live, info.NICs)
		} else {
			info.ActiveAdapter = "net-merge"
		}
		return info, nil
	}

	return info, nil
}

// SetSystemWide turns system-wide aggregation on or off.
func (c *Controller) SetSystemWide(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !on {
		c.stopLocalTunnelLocked()
		if _, ok := state.Read(c.stateDir); ok || platform.InstanceRunning() {
			return state.SendMarker(c.stateDir, "off")
		}
		return nil
	}

	// Turning ON:
	if c.tunOn {
		return nil
	}
	if _, ok := state.Read(c.stateDir); ok {
		return nil
	}

	c.applySchedulerModeLocked()
	_ = embedded.EnsureWintunDLL(c.stateDir)

	if c.config.EngineSource == "builtin" && platform.InstanceRunning() {
		if err := state.SendMarker(c.stateDir, "on"); err != nil {
			return err
		}
		if c.waitForActiveStatus(6 * time.Second) {
			return nil
		}
	}

	// 1. If built-in mode (default) and we have admin/root/CAP_NET_ADMIN privileges, start TUN in-process directly!
	if c.config.EngineSource == "builtin" && platform.IsAdmin() {
		if err := platform.CheckDriver(); err != nil {
			return fmt.Errorf("TUN driver check failed: %w", err)
		}
		t := tun.New(c.balancer)
		mtu := c.config.MTU
		if mtu < 1280 || mtu > 65535 {
			mtu = tun.DefaultMTU
		}
		t.MTU = mtu
		if err := t.On(); err != nil {
			return fmt.Errorf("failed to start built-in net-merge TUN adapter: %w", err)
		}
		c.tunDev = t
		c.tunOn = true
		c.tunSince = time.Now()
		modeDesc := "system-wide (Speed Mode)"
		if c.config.RoutingMode == "backup" {
			modeDesc = "system-wide (Backup Mode)"
		}
		state.Write(c.stateDir, state.Status{
			Version: nativeVersionLabel(), EngineSource: "builtin", RoutingMode: c.config.RoutingMode,
			PID:     os.Getpid(),
			Mode:    modeDesc,
			V6:      c.balancer.V6State(),
			Since:   c.tunSince,
			Updated: time.Now(),
			NICs:    c.sched.Snapshot(),
		})
		return nil
	}

	// 2. Built-in mode without admin privileges, or external binary mode:
	// Spawn the daemon (`run`) and wait for it to write active status.json.
	bin := c.resolve(c.config.EngineSource, c.config.BinaryPath)
	if !bin.IsValid || bin.Path == "" {
		return fmt.Errorf("net-merge binary is not valid: %s", bin.Error)
	}

	logPath := filepath.Join(c.stateDir, "net-merge.log")
	args := []string{
		"run",
		"-state-dir", c.stateDir,
		"-log", logPath,
		"-mtu", fmt.Sprintf("%d", c.config.MTU),
		"-ipv6", c.config.IPv6Mode,
		fmt.Sprintf("-auto-weight=%t", c.config.AutoWeight),
		"-rescan", fmt.Sprintf("%ds", c.config.RescanSec),
	}
	weights := make([]string, 0, len(c.config.Weights))
	for name, weight := range c.config.Weights {
		weights = append(weights, fmt.Sprintf("%s=%g", name, weight))
	}
	sort.Strings(weights)
	for _, weight := range weights {
		args = append(args, "-weight", weight)
	}
	if c.config.IncludeNICs != "" {
		args = append(args, "-include", c.config.IncludeNICs)
	}
	if bin.EngineSource == "builtin" {
		args = append(args,
			"-mode", c.config.RoutingMode, "-primary", c.config.PrimaryAdapter, "-secondary", c.config.SecondaryAdapter,
			"-scheduling", c.config.Scheduling, "-probe4", strings.Join(c.config.ProbeTargetsV4, ","), "-probe6", strings.Join(c.config.ProbeTargetsV6, ","),
		)
	} else if c.config.RoutingMode == "backup" {
		return fmt.Errorf("this engine supports Speed only; select the NetPilot engine for Backup Failover")
	}
	if runtime.GOOS == "windows" {
		args = append(args, "-hidden")
	}

	state.TakeMarker(c.stateDir, "off")
	_ = state.Prepare(c.stateDir)

	var launchErr error
	if runtime.GOOS == "windows" && !platform.IsAdmin() && bin.EngineSource == "builtin" {
		// Trigger UAC elevation directly on our single binary (`netpilot.exe run ...`)
		launchErr = platform.RelaunchElevated(args, true)
	} else if runtime.GOOS == "linux" && !platform.IsAdmin() {
		launchErr = startElevatedLinux(bin.Path, args, bin.EngineSource == "builtin")
	} else {
		cmd := exec.Command(bin.Path, args...)
		execx.PrepareCommand(cmd)
		launchErr = cmd.Start()
		if launchErr == nil {
			go cmd.Wait()
		}
	}

	if launchErr != nil {
		return fmt.Errorf("failed to launch net-merge engine: %w", launchErr)
	}

	if c.waitForActiveStatus(12 * time.Second) {
		if st, ok := state.Read(c.stateDir); ok {
			c.launchedPID, c.launched = st.PID, bin
			c.launchedMode = c.config.RoutingMode
			c.launchedSince = st.Since
		}
		return nil
	}

	if logBytes, err := os.ReadFile(logPath); err == nil && len(logBytes) > 0 {
		lines := strings.TrimSpace(string(logBytes))
		if len(lines) > 300 {
			lines = lines[len(lines)-300:]
		}
		return fmt.Errorf("net-merge engine did not become active: %s", lines)
	}
	return fmt.Errorf("net-merge engine did not start within timeout (check admin/UAC permissions or adapter connectivity)")
}

func (c *Controller) stopLocalTunnelLocked() {
	if c.tunDev != nil {
		c.tunDev.Close()
		c.tunDev = nil
	}
	if c.tunOn {
		c.tunOn = false
		state.Clear(c.stateDir)
	}
}

// StopOwned shuts down forwarding started by this desktop instance.
func (c *Controller) StopOwned() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	socksErr := c.stopSocks5Locked()
	c.stopLocalTunnelLocked()
	if st, ok := state.Read(c.stateDir); ok && c.launchedPID > 0 && st.PID == c.launchedPID && st.Since.Equal(c.launchedSince) {
		if err := state.SendMarker(c.stateDir, "off"); err != nil {
			return err
		}
	}
	return socksErr
}

func (c *Controller) waitForActiveStatus(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := state.Read(c.stateDir); ok {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// SetAutostart enables or disables launch at sign-in.
func (c *Controller) SetAutostart(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := platform.SetDesktopAutostart(enabled); err != nil {
		return err
	}
	c.config.Autostart = enabled
	c.cachedAuto = enabled
	c.lastAutoAt = time.Now()
	_ = c.saveConfigLocked()
	return nil
}

// SetLimit sets the maximum bandwidth in Mbit/s for a specific network adapter.
func (c *Controller) SetLimit(adapter string, mbps float64) error {
	if strings.TrimSpace(adapter) == "" || math.IsNaN(mbps) || mbps < 0 || mbps > 1e6 {
		return fmt.Errorf("provide an adapter and a speed cap between 0 and 1,000,000 Mbit/s")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	lims, err := state.LoadLimits(c.stateDir)
	if err != nil {
		return err
	}
	if mbps <= 0 {
		delete(lims, adapter)
	} else {
		lims[adapter] = mbps
	}
	if err := state.SaveLimits(c.stateDir, lims); err != nil {
		return err
	}
	if c.sched != nil {
		c.sched.SetLimit(adapter, mbps)
	}

	_ = state.SendLimit(c.stateDir, state.LimitRequest{
		Adapter: adapter,
		Mbps:    mbps,
	})
	return nil
}

// CleanupStale removes any leftover routes, gateway rules, or orphaned TUN devices.
func (c *Controller) CleanupStale() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocalTunnelLocked()
	state.Clear(c.stateDir)
	tun.CleanupStale()
	return nil
}

// StartSocks5 initializes the in-process SOCKS5 proxy balancer.
func (c *Controller) StartSocks5(listenAddr string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.socksRunning {
		return nil
	}
	if c.config.EngineSource != "builtin" {
		return fmt.Errorf("the in-app SOCKS proxy requires the NetPilot engine; switch engines in Settings first")
	}

	if listenAddr == "" {
		listenAddr = c.config.SocksAddress
		if listenAddr == "" {
			listenAddr = "127.0.0.1:1080"
		}
	}

	host, port, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return fmt.Errorf("invalid SOCKS5 address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip == nil || ip.To4() == nil || !ip.IsLoopback() {
		return fmt.Errorf("the SOCKS5 proxy requires an IPv4 loopback address such as 127.0.0.1:1080")
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("failed to bind SOCKS5 listener on %s: %w", listenAddr, err)
	}

	previous := c.config.SocksAddress
	c.config.SocksAddress = listenAddr
	if err := c.saveConfigLocked(); err != nil {
		c.config.SocksAddress = previous
		_ = ln.Close()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.socksLn = ln
	c.socksCancel = cancel
	c.socksRunning = true
	c.socksAddr = ln.Addr().String()
	go func() { _ = engine.ServeSocks5Context(ctx, ln, c.balancer) }()

	return nil
}

// StopSocks5 stops the running SOCKS5 proxy listener.
func (c *Controller) StopSocks5() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopSocks5Locked()
}

func (c *Controller) stopSocks5Locked() error {
	if !c.socksRunning || c.socksLn == nil {
		return nil
	}

	err := c.socksLn.Close()
	c.socksCancel()
	c.socksLn = nil
	c.socksCancel = nil
	c.socksRunning = false
	c.socksAddr = c.config.SocksAddress
	return err
}

// SetIPv6Mode updates the IPv6 balancing policy ("auto", "on", "off") immediately.
func (c *Controller) SetIPv6Mode(mode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v6, ok := engine.ParseV6Mode(mode)
	if !ok {
		v6 = engine.V6Auto
		mode = "auto"
	}
	c.config.IPv6Mode = mode
	if c.balancer != nil {
		c.balancer.SetV6Mode(v6)
	}
	return c.saveConfigLocked()
}

// SetAutoPause updates the auto-pause on single connection setting.
func (c *Controller) SetAutoPause(autoPause bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.AutoPause = autoPause
	return c.saveConfigLocked()
}

// SaveConfig updates and persists the net-merge configuration.
func (c *Controller) SaveConfig(cfg Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Engine selection goes through SetEngineSource, so a stale settings form
	// cannot change the engine behind the user's back.
	cfg.EngineSource, cfg.BinaryPath = c.config.EngineSource, c.config.BinaryPath
	cfg = normalizeConfig(cfg)
	previous := c.config
	c.config = cfg
	if err := c.saveConfigLocked(); err != nil {
		c.config = previous
		return err
	}
	c.applySchedulerModeLocked()
	if c.balancer != nil {
		if v6, ok := engine.ParseV6Mode(cfg.IPv6Mode); ok {
			c.balancer.SetV6Mode(v6)
		}
	}
	return nil
}

// GetConfig returns a copy of the net-merge configuration.
func (c *Controller) GetConfig() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

func (c *Controller) saveConfigLocked() error {
	if c.configFile == "" {
		return fmt.Errorf("could not locate the NetPilot user settings folder")
	}
	b, err := json.MarshalIndent(c.config, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.configFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".netpilot-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), c.configFile)
}
