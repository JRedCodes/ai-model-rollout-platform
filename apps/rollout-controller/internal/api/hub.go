package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// SSEHub manages connected SSE clients and broadcasts events to all of them.
type SSEHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func NewSSEHub() *SSEHub {
	return &SSEHub{clients: make(map[chan []byte]struct{})}
}

func (h *SSEHub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *SSEHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// Broadcast sends a typed event payload to all connected clients.
func (h *SSEHub) Broadcast(eventType string, data any) {
	payload, err := json.Marshal(map[string]any{"type": eventType, "data": data})
	if err != nil {
		log.Printf("sse: marshal error: %v", err)
		return
	}

	h.mu.Lock()
	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
			// client too slow; skip this event rather than blocking
		}
	}
	h.mu.Unlock()
}

// ServeHTTP implements http.Handler for GET /events.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
