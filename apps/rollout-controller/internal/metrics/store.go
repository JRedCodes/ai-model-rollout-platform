package metrics

import (
	"sort"
	"sync"
	"time"
)

type EventRecord struct {
	Timestamp time.Time
	Success   bool
	LatencyMs int
}

// Store is a thread-safe, time-ordered sliding window of inference event records.
// It prunes events older than 10 minutes but always retains at least 100 entries
// so the guard's fresh window is always satisfiable.
type Store struct {
	mu     sync.RWMutex
	events []EventRecord
}

func NewStore() *Store {
	return &Store{
		events: make([]EventRecord, 0, 1000),
	}
}

func (s *Store) Record(rec EventRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, rec)

	cutoff := time.Now().Add(-10 * time.Minute)
	pruneAt := 0
	for pruneAt < len(s.events) && s.events[pruneAt].Timestamp.Before(cutoff) {
		pruneAt++
	}

	const minRetain = 100
	if pruneAt > len(s.events)-minRetain {
		if len(s.events) > minRetain {
			pruneAt = len(s.events) - minRetain
		} else {
			pruneAt = 0
		}
	}

	if pruneAt > 0 {
		s.events = s.events[pruneAt:]
	}
}

func (s *Store) TotalCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// LastN returns a copy of the last n event records.
func (s *Store) LastN(n int) []EventRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := len(s.events) - n
	if start < 0 {
		start = 0
	}

	result := make([]EventRecord, len(s.events)-start)
	copy(result, s.events[start:])
	return result
}

// Since returns a copy of all events with a timestamp at or after t.
func (s *Store) Since(t time.Time) []EventRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	i := sort.Search(len(s.events), func(i int) bool {
		return !s.events[i].Timestamp.Before(t)
	})

	result := make([]EventRecord, len(s.events)-i)
	copy(result, s.events[i:])
	return result
}

// ErrorRate returns the fraction of failed events in the slice.
func ErrorRate(events []EventRecord) float64 {
	if len(events) == 0 {
		return 0
	}
	failures := 0
	for _, e := range events {
		if !e.Success {
			failures++
		}
	}
	return float64(failures) / float64(len(events))
}

// P95Latency returns the 95th-percentile latency in milliseconds for the slice.
func P95Latency(events []EventRecord) int {
	if len(events) == 0 {
		return 0
	}
	latencies := make([]int, len(events))
	for i, e := range events {
		latencies[i] = e.LatencyMs
	}
	sort.Ints(latencies)
	idx := int(float64(len(latencies)) * 0.95)
	if idx >= len(latencies) {
		idx = len(latencies) - 1
	}
	return latencies[idx]
}
