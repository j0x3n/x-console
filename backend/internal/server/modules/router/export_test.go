package router

import "time"

// Hooks for the external test package.

func SetNow(m *Module, now func() time.Time) { m.now = now }

// Parsers, to test them on their own.
var (
	ParseWifi  = parseWifi
	ParseNeigh = parseNeigh
	DeviceMAC  = deviceMAC
)

// ForgetDeviceMemory drops the in-memory device table, as a restart of the
// panel does. The next report reads it from the database again.
func ForgetDeviceMemory(m *Module) {
	m.devs.mu.Lock()
	m.devs.loaded, m.devs.byMAC = false, nil
	m.devs.mu.Unlock()
}

// Accessors for the unexported fields of a parsed neighbor entry.
func (n NeighEntry) MAC() string     { return n.mac }
func (n NeighEntry) Reachable() bool { return n.reachable() }
func (n NeighEntry) State() string   { return n.state }

type NeighEntry = neighEntry
