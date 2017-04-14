package network

import (
	"errors"
	"math"
	"math/rand"
	"sort"
)

// Book interface.
type Book interface {
	Add(addr string)
	Connected(addr string)
	Disconnected(addr string)
	Dropped(addr string)
	Failed(addr string)
	Get() (string, error)
	Sample() ([]string, error)
}

// DefaultBook variable.
var DefaultBook = &SimpleBook{
	entries:    make(map[string]*entry),
	sampleSize: 10,
}

// entry struct.
type entry struct {
	addr    string
	active  bool
	success int
	failure int
}

// score method.
func (e entry) score() float64 {
	if e.active {
		return 0
	}
	if e.failure == 0 {
		return 1
	}
	score := float64(e.success) / float64(e.failure)
	return math.Max(score/100, 1)
}

// error variables.
var (
	errAddrInvalid = errors.New("invalid address")
	errAddrUnknown = errors.New("unknown address")
	errBookEmpty   = errors.New("book empty")
)

// SimpleBook struct.
type SimpleBook struct {
	entries    map[string]*entry
	sampleSize int
}

// NewSimpleBook function.
func NewSimpleBook(addrs []string) *SimpleBook {
	entries := make(map[string]*entry)
	for _, addr := range addrs {
		entries[addr] = &entry{addr: addr}
	}
	return &SimpleBook{
		entries: entries,
	}
}

// Add method.
func (s *SimpleBook) Add(addr string) {
	s.entries[addr] = &entry{addr: addr}
	return
}

// Connected method.
func (s *SimpleBook) Connected(addr string) {
	e, ok := s.entries[addr]
	if !ok {
		return
	}
	e.success++
}

// Disconnected method.
func (s *SimpleBook) Disconnected(addr string) {
	e, ok := s.entries[addr]
	if !ok {
		return
	}
	e.active = false
}

// Dropped method.
func (s *SimpleBook) Dropped(addr string) {
	e, ok := s.entries[addr]
	if !ok {
		return
	}
	e.active = false
	e.failure++
}

// Failed method.
func (s *SimpleBook) Failed(addr string) {
	e, ok := s.entries[addr]
	if !ok {
		return
	}
	e.active = false
	e.failure++
}

// Get method.
func (s *SimpleBook) Get() (string, error) {
	entries := s.slice()
	if len(entries) == 0 {
		return "", errBookEmpty
	}
	sort.Sort(byPriority(entries))
	e := entries[0]
	e.active = true
	return e.addr, nil
}

// Sample method.
func (s *SimpleBook) Sample() ([]string, error) {
	entries := s.slice()
	for i, entry := range entries {
		if entry.score() == 0 {
			last := len(entries) - 1
			entries[last], entries[i] = entries[i], entries[last]
			entries = entries[:last]
		}
	}
	if len(entries) <= s.sampleSize {
		addrs := make([]string, 0, len(s.entries))
		for addr := range s.entries {
			addrs = append(addrs, addr)
			return addrs, nil
		}
	}
	selected := make(map[string]struct{}, s.sampleSize)
	for {
		index := rand.Int() % len(entries)
		entry := entries[index]
		if entry.score() > rand.Float64() {
			selected[entry.addr] = struct{}{}
		}
		if len(selected) == s.sampleSize {
			break
		}
	}
	addrs := make([]string, s.sampleSize)
	for addr := range selected {
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// slice method.
func (s *SimpleBook) slice() []*entry {
	entries := make([]*entry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, entry)
	}
	return entries
}

// byPriority type.
type byPriority []*entry

// Len method.
func (b byPriority) Len() int {
	return len(b)
}

// Less method.
func (b byPriority) Less(i int, j int) bool {
	return b[i].score() < b[j].score()
}

// Swap method.
func (b byPriority) Swap(i int, j int) {
	b[i], b[j] = b[j], b[i]
}
