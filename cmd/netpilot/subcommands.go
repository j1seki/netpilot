package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"netpilot/internal/branding"
	"netpilot/internal/engine"
	"netpilot/internal/icon"
	"netpilot/internal/logx"
	"netpilot/internal/netmerge"
	"netpilot/internal/nics"
	"netpilot/internal/platform"
	"netpilot/internal/state"
	"netpilot/internal/tray"
	"netpilot/internal/tun"
)

// maybeRunNetMergeSubcommand checks if NetPilot was invoked with a net-merge CLI subcommand
// (so the single NetPilot binary acts as netpilot GUI + net-merge CLI + net-merge-tray).
// Returns true if a subcommand was handled.
func maybeRunNetMergeSubcommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	cmd := args[1]
	rest := args[2:]

	switch cmd {
	case "version":
		fmt.Printf("net-merge %s (NetPilot %s single-binary engine)\n", netmerge.EmbeddedVersion(), branding.AppVersion)
		return true
	case "install", "uninstall":
		if err := platform.Install(cmd == "uninstall"); err != nil {
			nmDie(err)
		}
		return true
	case "autostart":
		if len(rest) != 1 || (rest[0] != "on" && rest[0] != "off" && rest[0] != "status") {
			fmt.Fprintln(os.Stderr, "usage: netpilot autostart on|off|status")
			os.Exit(2)
		}
		if err := platform.Autostart(rest[0]); err != nil {
			nmDie(err)
		}
		return true
	case "_icon":
		if len(rest) != 1 {
			os.Exit(2)
		}
		if err := os.WriteFile(rest[0], icon.ICO(true), 0o644); err != nil {
			nmDie(err)
		}
		return true
	case "tray":
		nmCmdTray(rest)
		return true
	case "run":
		nmCmdRun(rest)
		return true
	case "on", "off", "quit":
		nmCmdControl(cmd, rest)
		return true
	case "status":
		nmCmdStatus(rest)
		return true
	case "limit":
		nmCmdLimit(rest)
		return true
	case "socks":
		nmCmdSocks(rest)
		return true
	case "list":
		nmCmdList(rest)
		return true
	case "cleanup":
		nmCmdCleanup()
		return true
	default:
		return false
	}
}

type nmWeightFlags map[string]float64

func (w nmWeightFlags) String() string { return "" }
func (w nmWeightFlags) Set(v string) error {
	i := strings.LastIndex(v, "=")
	if i < 1 {
		return fmt.Errorf(`want "Name=3", got %q`, v)
	}
	n, err := strconv.ParseFloat(v[i+1:], 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("weight must be > 0 in %q", v)
	}
	w[v[:i]] = n
	return nil
}

type nmCommon struct {
	include   string
	weights   nmWeightFlags
	rescan    time.Duration
	stats     time.Duration
	auto      bool
	logFile   string
	stateDir  string
	ipv6      string
	mtu       int
	pprof     string
	mode      string
	primary   string
	secondary string
}

func (c *nmCommon) register(fs *flag.FlagSet) {
	c.weights = nmWeightFlags{}
	fs.StringVar(&c.include, "include", "", "comma-separated adapter names to use (default: all non-virtual)")
	fs.Var(c.weights, "weight", `fixed adapter weight, repeatable: -weight "Wi-Fi=3"`)
	fs.DurationVar(&c.rescan, "rescan", 3*time.Second, "adapter re-scan interval")
	fs.DurationVar(&c.stats, "stats", 5*time.Second, "stats print interval (0 = off)")
	fs.BoolVar(&c.auto, "auto-weight", true, "weight adapters by measured throughput (off when -weight is given)")
	fs.StringVar(&c.ipv6, "ipv6", "auto", "auto: IPv6 only if every adapter has it; on: always; off: never")
	fs.IntVar(&c.mtu, "mtu", tun.DefaultMTU, "MTU of the virtual adapter (1280-65535)")
	fs.StringVar(&c.pprof, "pprof", "", "serve Go's profiler on this loopback address, e.g. 127.0.0.1:6060")
	fs.StringVar(&c.logFile, "log", "", "write output to this file instead of the console")
	fs.StringVar(&c.stateDir, "state-dir", state.DefaultDir(), "where status/off marker files live")
	fs.StringVar(&c.mode, "mode", "speed", "routing mode: speed or backup")
	fs.StringVar(&c.primary, "primary", "", "primary adapter name when -mode=backup")
	fs.StringVar(&c.secondary, "secondary", "", "secondary/backup adapter name when -mode=backup")
}

func (c *nmCommon) nicsDetailed() ([]nics.Nic, []nics.Skip, error) {
	var inc []string
	for _, s := range strings.Split(c.include, ",") {
		if s = strings.TrimSpace(s); s != "" {
			inc = append(inc, s)
		}
	}
	found, skipped, err := nics.DiscoverNicsDetailed(inc, c.weights)
	if len(found) == 0 && len(inc) == 0 {
		ifaces, _ := net.Interfaces()
		var allNames []string
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp != 0 && ifc.Flags&net.FlagLoopback == 0 && ifc.Name != "net-merge" {
				allNames = append(allNames, ifc.Name)
			}
		}
		found, skipped, err = nics.DiscoverNicsDetailed(allNames, c.weights)
	}
	return found, skipped, err
}

func (c *nmCommon) nics() ([]nics.Nic, error) {
	ns, _, err := c.nicsDetailed()
	return ns, err
}

func (c *nmCommon) setup() error {
	if err := state.Prepare(c.stateDir); err != nil && platform.IsAdmin() {
		return err
	}
	if c.logFile != "" {
		f, err := os.OpenFile(c.logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		logx.SetOutput(f)
		platform.RedirectStd(f)
	}
	return nil
}

func nmStartPprof(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-pprof %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("-pprof must be a loopback address like 127.0.0.1:6060, got %q", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go http.Serve(ln, nil)
	logx.Printf("pprof on http://%s/debug/pprof/\n", addr)
	return nil
}

func (c *nmCommon) boot(mode func() string) (*engine.Scheduler, *engine.Balancer, error) {
	if c.mtu < 1280 || c.mtu > 65535 {
		return nil, nil, fmt.Errorf("-mtu must be between 1280 and 65535, got %d", c.mtu)
	}
	if c.pprof != "" {
		if err := nmStartPprof(c.pprof); err != nil {
			return nil, nil, err
		}
	}
	ns, err := c.nics()
	if err != nil {
		return nil, nil, err
	}
	if len(ns) == 0 {
		return nil, nil, fmt.Errorf("no usable IPv4 adapters found (try: netpilot list)")
	}
	sched := engine.NewScheduler(ns)
	sched.SetAuto(c.auto && len(c.weights) == 0)
	if strings.EqualFold(c.mode, "backup") {
		sched.SetMode(engine.ModeBackup, c.primary, c.secondary)
	} else {
		sched.SetMode(engine.ModeSpeed, c.primary, c.secondary)
	}
	if limits, err := state.LoadLimits(c.stateDir); err != nil {
		logx.Printf("warning: %v (ignored)\n", err)
	} else {
		sched.SetLimits(limits)
	}
	for _, n := range ns {
		logx.Printf("  + %s\n", nmDescribeNic(n))
	}

	bal := engine.NewBalancer(sched)
	v6, ok := engine.ParseV6Mode(c.ipv6)
	if !ok {
		return nil, nil, fmt.Errorf(`-ipv6 must be auto, on or off, got %q`, c.ipv6)
	}
	bal.SetV6Mode(v6)

	since := time.Now()
	go sched.RunProbes(nil)
	go func() {
		for range time.Tick(c.rescan) {
			if found, err := c.nics(); err == nil {
				sched.Update(found)
			}
		}
	}()
	go func() {
		last := time.Now()
		for now := range time.Tick(time.Second) {
			sched.Tick(now.Sub(last))
			last = now
			if mode != nil {
				if m := mode(); m != "" {
					rm, pri, sec := sched.Mode()
					if rm == engine.ModeBackup {
						m = m + " [backup"
						if pri != "" {
							m += ":" + pri
						}
						if sec != "" {
							m += "->" + sec
						}
						m += "]"
					}
					state.Write(c.stateDir, state.Status{
						PID:     os.Getpid(),
						Mode:    m,
						V6:      bal.V6State(),
						Since:   since,
						Updated: now,
						NICs:    sched.Snapshot(),
					})
				}
			}
		}
	}()
	if c.stats > 0 {
		go func() {
			for range time.Tick(c.stats) {
				logx.Printf("--- %s\n%s", time.Now().Format("15:04:05"), nmFormatNICs(sched.Snapshot()))
			}
		}()
	}
	return sched, bal, nil
}

func nmDescribeNic(n nics.Nic) string {
	s := n.Name
	if n.IP != nil {
		s += "  v4 " + n.IP.String()
	}
	if n.IP6 != nil {
		s += "  v6 " + n.IP6.String()
	}
	return s
}

func nmFormatNICs(snap []engine.SlotStats) string {
	var sb strings.Builder
	for _, s := range snap {
		gone := ""
		if !s.Present {
			gone = " (gone)"
		}
		lim := ""
		if s.LimitMbps > 0 {
			lim = "  limit " + nmLimitPhrase(s.LimitMbps)
		}
		fmt.Fprintf(&sb, "%-14s v4 %-11s v6 %-11s link down %6.1f up %5.1f Mbit/s  via net-merge %6.1f  active %3d  total %5d  w %.2f/%.2f  failed %d%s%s\n",
			s.Name, s.State, s.State6, s.LinkIn*8/1e6, s.LinkOut*8/1e6, (s.RateIn+s.RateOut)*8/1e6, s.Active, s.Total, s.Weight, s.Weight6, s.Failed, lim, gone)
	}
	return sb.String()
}

func nmDie(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	if !platform.HasConsole() {
		platform.ShowError(err.Error())
	}
	os.Exit(1)
}

func nmCmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var c nmCommon
	c.register(fs)
	fs.Parse(args)
	ns, skipped, err := c.nicsDetailed()
	if err != nil {
		nmDie(err)
	}
	if len(ns) == 0 {
		fmt.Println("No usable adapters found.")
	}
	for _, n := range ns {
		fmt.Printf("USE   %s  (index %d)\n", nmDescribeNic(n), n.Index)
	}
	for _, s := range skipped {
		fmt.Printf("skip  %s: %s\n", s.Name, s.Reason)
	}
}

func nmCmdSocks(args []string) {
	fs := flag.NewFlagSet("socks", flag.ExitOnError)
	var c nmCommon
	c.register(fs)
	listen := fs.String("listen", "127.0.0.1:1080", "SOCKS5 listen address")
	fs.Parse(args)
	if err := c.setup(); err != nil {
		nmDie(err)
	}
	_, bal, err := c.boot(nil)
	if err != nil {
		nmDie(err)
	}
	ln, err := net.Listen("tcp4", *listen)
	if err != nil {
		nmDie(err)
	}
	logx.Printf("net-merge SOCKS5 listening on %s\n", *listen)
	nmDie(engine.ServeSocks5(ln, bal))
}

func nmCmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var c nmCommon
	c.register(fs)
	hidden := fs.Bool("hidden", false, "when elevating, hide the new console window")
	crashAfter := fs.Duration("crash-after", 0, "test only: exit abruptly after this long")
	runFor := fs.Duration("for", 0, "turn off automatically after this long")
	fs.Parse(args)

	if !platform.IsAdmin() {
		child := append([]string{"run"}, args...)
		child = append(child, "-state-dir", c.stateDir)
		state.Prepare(c.stateDir)
		if err := platform.RelaunchElevated(child, *hidden); err != nil {
			nmDie(fmt.Errorf("could not start elevated: %w", err))
		}
		fmt.Println("Approve the UAC/admin prompt. System-wide mode runs in the background; `netpilot off` turns it off.")
		return
	}
	if !platform.SingleInstance() {
		fmt.Println("net-merge is already running.")
		return
	}
	if err := c.setup(); err != nil {
		nmDie(err)
	}
	if err := platform.CheckDriver(); err != nil {
		logx.Printf("error: %v\n", err)
		os.Exit(1)
	}
	state.TakeMarker(c.stateDir, "off")

	sched, bal, err := c.boot(func() string { return "system-wide" })
	if err != nil {
		logx.Printf("error: %v\n", err)
		os.Exit(1)
	}
	t := tun.New(bal)
	t.MTU = c.mtu
	if err := t.On(); err != nil {
		logx.Printf("error: %v\n", err)
		os.Exit(1)
	}
	logx.Printf("net-merge is ON: all IPv4 traffic is balanced across the adapters above.\n")

	// Write status immediately so callers waiting for activation see it right away
	state.Write(c.stateDir, state.Status{
		PID:     os.Getpid(),
		Mode:    "system-wide",
		V6:      bal.V6State(),
		Since:   time.Now(),
		Updated: time.Now(),
		NICs:    sched.Snapshot(),
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	var crash, timeUp <-chan time.Time
	if *crashAfter > 0 {
		crash = time.After(*crashAfter)
	}
	if *runFor > 0 {
		timeUp = time.After(*runFor)
	}
	poll := time.NewTicker(300 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-sig:
		case <-timeUp:
		case <-crash:
			os.Exit(3)
		case <-poll.C:
			if req, ok := state.TakeLimit(c.stateDir); ok {
				sched.SetLimit(req.Adapter, req.Mbps)
				if err := state.SaveLimits(c.stateDir, sched.Limits()); err != nil {
					logx.Printf("warning: saving speed limits: %v\n", err)
				}
			}
			if !state.TakeMarker(c.stateDir, "off") && !state.TakeMarker(c.stateDir, "quit") {
				continue
			}
		}
		break
	}
	t.Close()
	state.Clear(c.stateDir)
	logx.Printf("net-merge is OFF.\n")
}

func nmCmdTray(args []string) {
	fs := flag.NewFlagSet("tray", flag.ExitOnError)
	var c nmCommon
	c.register(fs)
	hidden := fs.Bool("hidden", true, "hide the console window of the elevated process")
	startOff := fs.Bool("off", false, "start with system-wide mode OFF (default: ON)")
	autoPause := fs.Bool("auto-pause", true, "pause when fewer than two connections work")
	fs.Parse(args)

	if !platform.IsAdmin() {
		child := append([]string{"tray"}, args...)
		if c.stateDir != state.DefaultDir() {
			child = append(child, "-state-dir", c.stateDir)
		}
		if err := platform.RelaunchTray(child, *hidden); err != nil {
			nmDie(err)
		}
		return
	}
	if !platform.SingleInstance() {
		return
	}
	if c.logFile == "" {
		c.stats = 0
		if !platform.HasConsole() {
			c.logFile = filepath.Join(c.stateDir, "net-merge.log")
		}
	}
	if err := c.setup(); err != nil {
		nmDie(err)
	}
	if err := platform.CheckDriver(); err != nil {
		nmDie(err)
	}

	t := tun.New(nil)
	t.MTU = c.mtu
	mode := func() string {
		if t.IsOn() {
			return "system-wide (tray)"
		}
		return ""
	}
	sched, bal, err := c.boot(mode)
	if err != nil {
		nmDie(err)
	}
	t.B = bal

	if !platform.WaitForTray(3 * time.Minute) {
		logx.Printf("warning: taskbar not found after 3 minutes, trying anyway\n")
	}
	ready := make(chan struct{})
	go func() {
		select {
		case <-ready:
		case <-time.After(30 * time.Second):
			os.Exit(4)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		t.Close()
		state.Clear(c.stateDir)
		os.Exit(0)
	}()
	tray.Run(t, sched, bal, c.stateDir, netmerge.EmbeddedVersion(), !*startOff, *autoPause, ready)
}

func nmCmdControl(name string, args []string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	dir := fs.String("state-dir", state.DefaultDir(), "state directory")
	fs.Parse(args)
	if !platform.InstanceRunning() {
		if name == "off" || name == "quit" {
			fmt.Println("net-merge is not running (already off).")
			return
		}
		nmDie(fmt.Errorf("net-merge is not running"))
	}
	if err := state.SendMarker(*dir, name); err != nil {
		nmDie(err)
	}
	want := name == "on"
	for i := 0; i < 60; i++ {
		time.Sleep(150 * time.Millisecond)
		_, on := state.Read(*dir)
		switch {
		case name == "on" && on == want:
			fmt.Println("net-merge is ON.")
			return
		case name != "on" && !on:
			fmt.Println("net-merge is OFF.")
			return
		}
	}
	nmDie(fmt.Errorf("sent %q but it did not take effect", name))
}

func nmCmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	dir := fs.String("state-dir", state.DefaultDir(), "state directory")
	fs.Parse(args)
	st, on := state.Read(*dir)
	if !on {
		fmt.Println("net-merge is OFF.")
		return
	}
	fmt.Printf("net-merge is ON (%s, pid %d, up %s)\nIPv6: %s\n%s", st.Mode, st.PID, time.Since(st.Since).Truncate(time.Second), st.V6, nmFormatNICs(st.NICs))
}

func nmCmdLimit(args []string) {
	fs := flag.NewFlagSet("limit", flag.ExitOnError)
	dir := fs.String("state-dir", state.DefaultDir(), "state directory")
	fs.Parse(args)
	rest := fs.Args()

	if len(rest) == 0 {
		limits, err := state.LoadLimits(*dir)
		if err != nil {
			nmDie(err)
		}
		if len(limits) == 0 {
			fmt.Println("No speed limits set.")
			return
		}
		names := make([]string, 0, len(limits))
		for name := range limits {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("%-14s %s\n", name, nmLimitPhrase(limits[name]))
		}
		return
	}
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: netpilot limit <adapter> <Mbit/s>")
		os.Exit(2)
	}
	name := strings.TrimSpace(rest[0])
	mbitPerSec, err := engine.ParseLimit(rest[1])
	if err != nil {
		nmDie(err)
	}
	if !platform.InstanceRunning() {
		limits, err := state.LoadLimits(*dir)
		if err != nil {
			nmDie(err)
		}
		if mbitPerSec <= 0 {
			delete(limits, name)
		} else {
			limits[name] = mbitPerSec
		}
		if err := state.SaveLimits(*dir, limits); err != nil {
			nmDie(err)
		}
		fmt.Printf("%s: %s.\n", name, nmLimitPhrase(mbitPerSec))
		return
	}
	if err := state.SendLimit(*dir, state.LimitRequest{Adapter: name, Mbps: mbitPerSec}); err != nil {
		nmDie(err)
	}
	fmt.Printf("%s: %s.\n", name, nmLimitPhrase(mbitPerSec))
}

func nmLimitPhrase(mbitPerSec float64) string {
	if mbitPerSec <= 0 {
		return "unlimited"
	}
	return strconv.FormatFloat(mbitPerSec, 'f', -1, 64) + " Mbit/s"
}

func nmCmdCleanup() {
	if !platform.IsAdmin() {
		if err := platform.RelaunchElevated([]string{"cleanup"}, false); err != nil {
			nmDie(err)
		}
		return
	}
	tun.CleanupStale()
	fmt.Println("done.")
}
