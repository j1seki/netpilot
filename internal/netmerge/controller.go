package netmerge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"netpilot/internal/engine"
	"netpilot/internal/execx"
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
	EngineSource     string             `json:"engineSource"`     // "builtin" (default) or "external"
	BinaryPath       string             `json:"binaryPath,omitempty"`
	RoutingMode      string             `json:"routingMode"`      // "speed" (default) or "backup"
	PrimaryAdapter   string             `json:"primaryAdapter"`   // primary NIC name in backup mode
	SecondaryAdapter string             `json:"secondaryAdapter"` // secondary/failover NIC name in backup mode
	CloseAction      string             `json:"closeAction"`      // "minimize" (default) or "close"
}

// StatusInfo represents the runtime state of net-merge.
type StatusInfo struct {
	IsRunning        bool               `json:"isRunning"`
	PID              int                `json:"pid"`
	Mode             string             `json:"mode"`
	RoutingMode      string             `json:"routingMode"`      // "speed" or "backup"
	DefaultAdapter   string             `json:"defaultAdapter"`   // "net-merge" when active, "System Default" when inactive
	ActiveAdapter    string             `json:"activeAdapter"`    // currently active primary/failover adapter or "net-merge"
	V6State          string             `json:"v6State"`
	UptimeSec        int64              `json:"uptimeSec"`
	Autostart        bool               `json:"autostart"`
	SocksRunning     bool               `json:"socksRunning"`
	SocksAddress     string             `json:"socksAddress"`
	Config           Config             `json:"config"`
	NICs             []engine.SlotStats `json:"nics"`
	Limits           map[string]float64 `json:"limits"`
	Binary           BinaryInfo         `json:"binary"`
}

// Controller coordinates all net-merge operations directly inside NetPilot.
type Controller struct {
	mu           sync.RWMutex
	stateDir     string
	configFile   string
	config       Config
	socksLn      net.Listener
	socksRunning bool
	socksAddr    string

	// In-process scheduler and balancer
	sched      *engine.Scheduler
	balancer   *engine.Balancer
	tunDev     *tun.Tun
	tunOn      bool
	tunSince   time.Time
	stopCh     chan struct{}
	cachedAuto bool
	lastAutoAt time.Time
}

// NewController creates and initializes the Net-Merge controller with a live scheduler.
func NewController(stateDir string) *Controller {
	if stateDir == "" {
		stateDir = state.DefaultDir()
	}
	_ = state.Prepare(stateDir)

	cfgPath := filepath.Join(stateDir, "netpilot-netmerge-config.json")
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
	if cfg.EngineSource != "external" {
		if strings.TrimSpace(cfg.BinaryPath) != "" && cfg.EngineSource == "" {
			cfg.EngineSource = "external"
		} else {
			cfg.EngineSource = "builtin"
		}
	}
	if cfg.RoutingMode != "backup" {
		cfg.RoutingMode = "speed"
	}
	if cfg.CloseAction != "close" {
		cfg.CloseAction = "minimize"
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

func (c *Controller) discover() []nics.Nic {
	inc := c.parseIncludes()
	found, _ := nics.DiscoverNics(inc, c.config.Weights)
	if len(found) == 0 && len(inc) == 0 {
		// Fallback for virtualized environments where default hardware filter skips eth0
		ifaces, _ := net.Interfaces()
		var allNames []string
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp != 0 && ifc.Flags&net.FlagLoopback == 0 && ifc.Name != "net-merge" {
				allNames = append(allNames, ifc.Name)
			}
		}
		found, _ = nics.DiscoverNics(allNames, c.config.Weights)
	}
	return found
}

func (c *Controller) applySchedulerModeLocked() {
	if c.sched == nil {
		return
	}
	c.sched.SetAuto(c.config.AutoWeight && len(c.config.Weights) == 0)
	if c.config.RoutingMode == "backup" {
		c.sched.SetMode(engine.ModeBackup, c.config.PrimaryAdapter, c.config.SecondaryAdapter)
	} else {
		c.sched.SetMode(engine.ModeSpeed, c.config.PrimaryAdapter, c.config.SecondaryAdapter)
	}
}

func (c *Controller) initScheduler() {
	initial := c.discover()
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
				found := c.discover()
				if len(found) > 0 {
					c.sched.Update(found)
				}
			case now := <-tickTicker.C:
				c.sched.Tick(now.Sub(last))
				last = now
				c.mu.RLock()
				isOn := c.tunOn
				since := c.tunSince
				rm := c.config.RoutingMode
				c.mu.RUnlock()
				if isOn {
					modeDesc := "system-wide (Speed Mode)"
					if rm == "backup" {
						modeDesc = "system-wide (Backup Failover)"
					}
					state.Write(c.stateDir, state.Status{
						PID:     os.Getpid(),
						Mode:    modeDesc,
						V6:      c.balancer.V6State(),
						Since:   since,
						Updated: now,
						NICs:    c.sched.Snapshot(),
					})
				}
			}
		}
	}()
}

func (c *Controller) checkAutostartCached() bool {
	if time.Since(c.lastAutoAt) > 30*time.Second {
		c.cachedAuto = platform.AutostartEnabled()
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
	if strings.EqualFold(strings.TrimSpace(mode), "backup") {
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
	c.applySchedulerModeLocked()
	return c.saveConfigLocked()
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
	inProcOn := c.tunOn
	inProcSince := c.tunSince
	cfg := c.config
	c.mu.Unlock()

	c.mu.RLock()
	defer c.mu.RUnlock()

	snap := []engine.SlotStats{}
	if c.sched != nil {
		snap = c.sched.Snapshot()
	}

	binInfo := ResolveBinaryWithSource(cfg.EngineSource, cfg.BinaryPath, c.stateDir)

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
		info.V6State = st.V6
		info.UptimeSec = int64(time.Since(st.Since).Seconds())
		if len(st.NICs) > 0 {
			info.NICs = st.NICs
		}
		info.DefaultAdapter = "net-merge"
		info.ActiveAdapter = resolveActiveAdapter(true, cfg, info.NICs)
		return info, nil
	}

	return info, nil
}

// SetSystemWide turns system-wide aggregation on or off.
func (c *Controller) SetSystemWide(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !on {
		// Turn off in-process TUN if active
		if c.tunDev != nil {
			c.tunDev.Off()
			c.tunDev.Close()
			c.tunDev = nil
		}
		c.tunOn = false
		state.Clear(c.stateDir)

		// Signal external binary if running
		_ = state.SendMarker(c.stateDir, "off")
		if platform.InstanceRunning() {
			bin := ResolveBinaryWithSource(c.config.EngineSource, c.config.BinaryPath, c.stateDir)
			if bin.IsValid && bin.Path != "" {
				exe, _ := os.Executable()
				if bin.Path != exe {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					cmd := exec.CommandContext(ctx, bin.Path, "off", "-state-dir", c.stateDir)
					execx.PrepareCommand(cmd)
					_ = cmd.Run()
					cancel()
				}
			}
		}
		return nil
	}

	// Turning ON:
	if c.tunOn {
		return nil
	}

	c.applySchedulerModeLocked()

	// 1. If user explicitly selected an external net-merge binary, or if an external instance is already running:
	bin := ResolveBinaryWithSource(c.config.EngineSource, c.config.BinaryPath, c.stateDir)
	exe, _ := os.Executable()

	if platform.InstanceRunning() {
		_ = state.SendMarker(c.stateDir, "on")
		return nil
	}

	// 2. If built-in mode (default) and we have admin/root privileges, start TUN in-process directly!
	if c.config.EngineSource != "external" && platform.IsAdmin() {
		if err := platform.CheckDriver(); err == nil {
			t := tun.New(c.balancer)
			mtu := c.config.MTU
			if mtu < 1280 || mtu > 65535 {
				mtu = tun.DefaultMTU
			}
			t.MTU = mtu
			if err := t.On(); err == nil {
				c.tunDev = t
				c.tunOn = true
				c.tunSince = time.Now()
				modeDesc := "system-wide (Speed Mode)"
				if c.config.RoutingMode == "backup" {
					modeDesc = "system-wide (Backup Mode)"
				}
				state.Write(c.stateDir, state.Status{
					PID:     os.Getpid(),
					Mode:    modeDesc,
					V6:      c.balancer.V6State(),
					Since:   c.tunSince,
					Updated: time.Now(),
					NICs:    c.sched.Snapshot(),
				})
				return nil
			}
		}
	}

	// 3. Launch external or extracted embedded net-merge binary if available (and distinct from NetPilot GUI exe)
	if bin.IsValid && bin.Path != "" && bin.Path != exe {
		args := []string{
			"run",
			"-state-dir", c.stateDir,
			"-mode", c.config.RoutingMode,
		}
		if c.config.PrimaryAdapter != "" {
			args = append(args, "-primary", c.config.PrimaryAdapter)
		}
		if c.config.SecondaryAdapter != "" {
			args = append(args, "-secondary", c.config.SecondaryAdapter)
		}
		if runtime.GOOS == "windows" {
			args = append(args, "-hidden")
			if bin.TrayPath != "" && fileExists(bin.TrayPath) {
				cmd := exec.Command(bin.TrayPath, "-state-dir", c.stateDir, "-mode", c.config.RoutingMode, "-primary", c.config.PrimaryAdapter, "-secondary", c.config.SecondaryAdapter)
				execx.PrepareCommand(cmd)
				if err := cmd.Start(); err == nil {
					_ = state.SendMarker(c.stateDir, "on")
					return nil
				}
			}
		}
		cmd := exec.Command(bin.Path, args...)
		execx.PrepareCommand(cmd)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}

	// 4. Activate multi-link balancer state in-process so NetPilot's net-merge adapter state is live
	c.tunOn = true
	c.tunSince = time.Now()
	modeDesc := "system-wide (Speed Mode)"
	if c.config.RoutingMode == "backup" {
		modeDesc = "system-wide (Backup Mode)"
	}
	state.Write(c.stateDir, state.Status{
		PID:     os.Getpid(),
		Mode:    modeDesc,
		V6:      c.balancer.V6State(),
		Since:   c.tunSince,
		Updated: time.Now(),
		NICs:    c.sched.Snapshot(),
	})
	_ = state.SendMarker(c.stateDir, "on")
	return nil
}

// SetAutostart enables or disables launch at sign-in.
func (c *Controller) SetAutostart(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := platform.SetAutostart(enabled); err != nil {
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sched != nil {
		c.sched.SetLimit(adapter, mbps)
	}

	lims, _ := state.LoadLimits(c.stateDir)
	if lims == nil {
		lims = make(map[string]float64)
	}
	if mbps <= 0 {
		delete(lims, adapter)
	} else {
		lims[adapter] = mbps
	}
	if err := state.SaveLimits(c.stateDir, lims); err != nil {
		return err
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
	if c.tunDev != nil {
		c.tunDev.Off()
		c.tunDev.Close()
		c.tunDev = nil
	}
	c.tunOn = false
	c.mu.Unlock()
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

	if listenAddr == "" {
		listenAddr = c.config.SocksAddress
		if listenAddr == "" {
			listenAddr = "127.0.0.1:1080"
		}
	}

	ln, err := net.Listen("tcp4", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to bind SOCKS5 listener on %s: %w", listenAddr, err)
	}

	go func() {
		_ = engine.ServeSocks5(ln, c.balancer)
	}()

	c.socksLn = ln
	c.socksRunning = true
	c.socksAddr = listenAddr
	c.config.SocksAddress = listenAddr
	_ = c.saveConfigLocked()

	return nil
}

// StopSocks5 stops the running SOCKS5 proxy listener.
func (c *Controller) StopSocks5() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.socksRunning || c.socksLn == nil {
		return nil
	}

	err := c.socksLn.Close()
	c.socksLn = nil
	c.socksRunning = false
	return err
}

// SaveConfig updates and persists the net-merge configuration.
func (c *Controller) SaveConfig(cfg Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg = normalizeConfig(cfg)
	c.config = cfg
	c.applySchedulerModeLocked()
	if c.balancer != nil {
		if v6, ok := engine.ParseV6Mode(cfg.IPv6Mode); ok {
			c.balancer.SetV6Mode(v6)
		}
	}
	return c.saveConfigLocked()
}

// GetConfig returns a copy of the net-merge configuration.
func (c *Controller) GetConfig() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

func (c *Controller) saveConfigLocked() error {
	b, err := json.MarshalIndent(c.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configFile, b, 0o644)
}
