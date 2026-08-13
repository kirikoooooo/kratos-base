package service

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type CLIOutputEvent struct {
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Text      string    `json:"text,omitempty"`
	Time      time.Time `json:"time"`
}

type CLIOutputStream struct {
	mu   sync.Mutex
	subs map[chan CLIOutputEvent]struct{}
}

func NewCLIOutputStream() *CLIOutputStream {
	return &CLIOutputStream{subs: make(map[chan CLIOutputEvent]struct{})}
}

func (s *CLIOutputStream) Publish(event CLIOutputEvent) {
	if s == nil || strings.TrimSpace(event.SessionID) == "" || strings.TrimSpace(event.Type) == "" {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *CLIOutputStream) subscribe() (<-chan CLIOutputEvent, func()) {
	ch := make(chan CLIOutputEvent, 32)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
}

func RegisterCLIOutputStream(mux interface {
	Handle(string, http.Handler)
}, stream *CLIOutputStream) {
	mux.Handle("/debug/cli/stream", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := prepareSSE(w)
		if !ok {
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id is required"})
			return
		}
		ch, cancel := stream.subscribe()
		defer cancel()
		if !writeSSE(w, flusher, "ready", map[string]string{"session_id": sessionID}) {
			return
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case event, open := <-ch:
				if !open {
					return
				}
				if event.SessionID == sessionID && !writeSSE(w, flusher, event.Type, event) {
					return
				}
			}
		}
	}))
}
