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
	YtDlpPath     string `json:"ytDlpPath"`
	// YtDlpCheckedAt records the last self-update check, so it runs once a day.
	YtDlpCheckedAt string `json:"ytDlpCheckedAt"`
	MaxHeight      int    `json:"maxHeight"`
	// CookieSource is a browser name for --cookies-from-browser, or "file:<path>"
	// for a cookies.txt export. Needed by sites the user is logged into.
	CookieSource string `json:"cookieSource"`
	// Advanced yt-dlp switches for the YouTube PO token provider.
	PluginDir       string `json:"pluginDir"`
	POTokenProvider string `json:"poTokenProvider"`
	POToken         string `json:"poToken"`
	PlayerClient    string `json:"playerClient"`
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

func (s *settingsStore) setYtDlpPath(value string) error {
	s.mu.Lock()
	s.data.YtDlpPath = value
	s.mu.Unlock()
	return s.save()
}

func (s *settingsStore) setYtDlpCheckedAt(value string) error {
	s.mu.Lock()
	s.data.YtDlpCheckedAt = value
	s.mu.Unlock()
	return s.save()
}

func (s *settingsStore) setCookieSource(value string) error {
	s.mu.Lock()
	s.data.CookieSource = value
	s.mu.Unlock()
	return s.save()
}

func (s *settingsStore) setYtDlpTuning(pluginDir, provider, token, playerClient string) error {
	s.mu.Lock()
	s.data.PluginDir = pluginDir
	s.data.POTokenProvider = provider
	s.data.POToken = token
	s.data.PlayerClient = playerClient
	s.mu.Unlock()
	return s.save()
}

func (s *settingsStore) setMaxHeight(value int) error {
	s.mu.Lock()
	s.data.MaxHeight = value
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
