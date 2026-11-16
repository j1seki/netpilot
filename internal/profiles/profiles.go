// Package profiles stores named NetPilot configurations (routing mode, scheduling, MTU,
// IPv6 and hotspot profile) so a user can switch between "Gaming", "Travel" and similar
// setups from the UI or the tray without re-entering every setting.
package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"netpilot/internal/netmerge"
)

// Profile is a saved configuration.
type Profile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	NetMerge    netmerge.Config `json:"netMerge"`
	Hotspot     string          `json:"hotspotProfile,omitempty"`
	Builtin     bool            `json:"builtin,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Store persists user-created profiles as JSON in the NetPilot state directory.
type Store struct {
	mu    sync.Mutex
	path  string
	items []Profile
}

// Open loads the profile file. A missing file is an empty store, not an error.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("profiles: state directory is empty")
	}
	s := &Store{path: filepath.Join(dir, "netpilot-profiles.json")}
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(b, &s.items); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// List returns the user profiles sorted by name.
func (s *Store) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Profile(nil), s.items...)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Get finds a profile by ID.
func (s *Store) Get(id string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.items {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// Save inserts or updates a profile and persists it.
func (s *Store) Save(p Profile) (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Profile{}, errors.New("profile name is required")
	}
	if strings.HasPrefix(p.ID, "builtin-") {
		return Profile{}, errors.New("profile id is reserved for built-ins")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	p.Builtin = false
	if p.ID == "" {
		p.ID = newID()
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	found := false
	for i := range s.items {
		if s.items[i].ID == p.ID {
			p.CreatedAt = s.items[i].CreatedAt
			s.items[i] = p
			found = true
			break
		}
	}
	if !found {
		s.items = append(s.items, p)
	}
	if err := s.flushLocked(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Delete removes a user profile.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return s.flushLocked()
		}
	}
	return errors.New("profile not found")
}

func (s *Store) flushLocked() error {
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "p-" + hex.EncodeToString(b[:])
}

// Builtins returns virtual profiles derived from the current configuration. They are not
// persisted: a built-in always applies on top of the current settings, so it can never
// silently revert fields the user changed later.
func Builtins(current netmerge.Config) []Profile {
	mk := func(id, name, desc string, mutate func(*netmerge.Config)) Profile {
		cfg := current
		cfg.Weights = cloneWeights(current.Weights)
		mutate(&cfg)
		return Profile{ID: id, Name: name, Description: desc, NetMerge: cfg, Builtin: true}
	}
	return []Profile{
		mk("builtin-max-speed", "Max Download", "Spread every flow across all healthy adapters.", func(c *netmerge.Config) {
			c.RoutingMode = "speed"
			c.Scheduling = "balanced"
			c.IPv6Mode = "auto"
		}),
		mk("builtin-low-latency", "Low Latency", "Prefer the adapter with the lowest probe RTT and jitter.", func(c *netmerge.Config) {
			c.RoutingMode = "speed"
			c.Scheduling = "low-latency"
		}),
		mk("builtin-backup", "Backup Failover", "Use the primary adapter and switch only when it fails.", func(c *netmerge.Config) {
			c.RoutingMode = "backup"
			c.Scheduling = "balanced"
		}),
		mk("builtin-hotspot", "Hotspot Saver", "IPv4-only, 1420 MTU for phone tethering.", func(c *netmerge.Config) {
			c.RoutingMode = "speed"
			c.Scheduling = "balanced"
			c.IPv6Mode = "off"
			c.MTU = 1420
		}),
	}
}

func cloneWeights(in map[string]float64) map[string]float64 {
	if in == nil {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
