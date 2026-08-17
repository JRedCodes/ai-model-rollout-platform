package metrics

import "testing"

func TestErrorRate(t *testing.T) {
	cases := []struct {
		name   string
		events []EventRecord
		want   float64
	}{
		{
			name:   "empty window",
			events: nil,
			want:   0,
		},
		{
			name:   "single success",
			events: []EventRecord{{Success: true}},
			want:   0,
		},
		{
			name:   "single failure",
			events: []EventRecord{{Success: false}},
			want:   1,
		},
		{
			name: "mixed",
			events: []EventRecord{
				{Success: true}, {Success: true}, {Success: true}, {Success: false},
			},
			want: 0.25,
		},
		{
			name: "all failures",
			events: []EventRecord{
				{Success: false}, {Success: false},
			},
			want: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ErrorRate(c.events)
			if got != c.want {
				t.Fatalf("ErrorRate(%d events) = %v, want %v", len(c.events), got, c.want)
			}
		})
	}
}

func TestP95Latency(t *testing.T) {
	cases := []struct {
		name   string
		events []EventRecord
		want   int
	}{
		{
			name:   "empty window",
			events: nil,
			want:   0,
		},
		{
			name:   "single sample",
			events: []EventRecord{{LatencyMs: 42}},
			want:   42,
		},
		{
			name: "two samples, P95 lands on the higher one",
			events: []EventRecord{
				{LatencyMs: 10}, {LatencyMs: 20},
			},
			want: 20,
		},
		{
			name: "twenty samples, P95 is the max (index 19 of 20)",
			events: func() []EventRecord {
				events := make([]EventRecord, 20)
				for i := range events {
					events[i] = EventRecord{LatencyMs: (i + 1) * 10} // 10, 20, ..., 200
				}
				return events
			}(),
			want: 200,
		},
		{
			name: "unsorted input is sorted before computing the percentile",
			events: []EventRecord{
				{LatencyMs: 300}, {LatencyMs: 100}, {LatencyMs: 200},
			},
			want: 300,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := P95Latency(c.events)
			if got != c.want {
				t.Fatalf("P95Latency(%d events) = %v, want %v", len(c.events), got, c.want)
			}
		})
	}
}

func TestP95LatencyDoesNotMutateInputOrder(t *testing.T) {
	events := []EventRecord{
		{LatencyMs: 300}, {LatencyMs: 100}, {LatencyMs: 200},
	}
	original := append([]EventRecord(nil), events...)

	P95Latency(events)

	for i := range events {
		if events[i] != original[i] {
			t.Fatalf("P95Latency mutated its input at index %d: got %+v, want %+v", i, events[i], original[i])
		}
	}
}
