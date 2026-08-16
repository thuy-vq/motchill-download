package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type appSettings struct {
	LastOutputDir string `json:"lastOutputDir"`
	FFmpegPath    string `json:"ffmpegPath"`
}

type settingsStore struct {
	mu   sync.Mutex
	path string
	data appSettings
}

func newSettingsStore() *settingsStore {
	configRoot, err := os.UserConfigDir()
	if err != nil || configRoot == "" {
		configRoot = os.TempDir()
	}
	store := &settingsStore{path: filepath.Join(configRoot, "MotchillDownloader", "settings.json")}
	store.load()
	if store.data.LastOutputDir == "" {
		if videos, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(videos, "Videos")
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				store.data.LastOutputDir = candidate
			} else {
				store.data.LastOutputDir = videos
			}
		}
	}
	return store
}

func (s *settingsStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, &s.data)
	}
}

func (s *settingsStore) snapshot() appSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

func (s *settingsStore) setOutputDir(value string) error {
	s.mu.Lock()
	s.data.LastOutputDir = value
	s.mu.Unlock()
	return s.save()
}

func (s *settingsStore) setFFmpegPath(value string) error {
	s.mu.Lock()
	s.data.FFmpegPath = value
	s.mu.Unlock()
	return s.save()
}

func (s *settingsStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
