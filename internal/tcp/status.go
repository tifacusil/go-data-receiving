package tcp

import "sync/atomic"

// Status tracks whether the TCP listener is up. Read by the health endpoint
// (H4); the C# equivalent is the TcpServerStatus singleton, made concurrency-safe
// here with an atomic since multiple goroutines touch it.
type Status struct {
	listening atomic.Bool
}

// NewStatus returns a Status initialized to not-listening.
func NewStatus() *Status { return &Status{} }

// SetListening records the listening state.
func (s *Status) SetListening(v bool) { s.listening.Store(v) }

// IsListening reports whether the listener is accepting connections.
func (s *Status) IsListening() bool { return s.listening.Load() }
