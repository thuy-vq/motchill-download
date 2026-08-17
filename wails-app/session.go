package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// sessionStore keeps the last movie list and its per-episode results on disk so
// an interrupted queue can be reopened after the app restarts.
type sessionStore struct {
	mu   sync.Mutex
	path string
}

func newSessionStore() *sessionStore {
	configRoot, err := os.UserConfigDir()
	if err != nil || configRoot == "" {
		configRoot = os.TempDir()
	}
	return &sessionStore{path: filepath.Join(configRoot, "MotchillDownloader", "session.json")}
}

func (s *sessionStore) save(state SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.Version = appVersion
	state.SavedAt = time.Now().Format(time.RFC3339)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	// Write beside the target first so a crash cannot leave a half file behind.
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, s.path)
}

func (s *sessionStore) load() (SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionState{}, nil
		}
		return SessionState{}, err
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		// A corrupt file must not block startup; treat it as "no session".
		return SessionState{}, nil
	}
	return state, nil
}

func (s *sessionStore) clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// summarize counts the outcome of a saved session. Only selected episodes can be
// pending, because unselected ones were never meant to be downloaded.
func summarizeSession(state SessionState) SessionSummary {
	summary := SessionSummary{SavedAt: state.SavedAt, Version: state.Version, Movies: len(state.Movies)}
	for _, movie := range state.Movies {
		summary.Episodes += len(movie.Episodes)
		for _, episode := range movie.Episodes {
			switch episode.Status {
			case "completed":
				summary.Completed++
			case "failed":
				summary.Failed++
			case "skipped":
				summary.Skipped++
			default:
				if episode.Selected {
					summary.Pending++
				}
			}
		}
	}
	summary.NeedsAttention = summary.Failed > 0 || summary.Pending > 0
	return summary
}
