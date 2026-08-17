package main

type MediaStream struct {
	URL    string `json:"url"`
	Kind   string `json:"kind"`
	Server string `json:"server"`
}

type Episode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Number  int    `json:"number"`
	PageURL string `json:"pageUrl"`
	// StreamURL is the first choice; Streams carries every server the host
	// offers for this episode so a dead link can fall back to another.
	StreamURL string        `json:"streamUrl,omitempty"`
	Streams   []MediaStream `json:"streams,omitempty"`
	Current   bool          `json:"current"`
}

type AnalysisResult struct {
	Title       string        `json:"title"`
	PageURL     string        `json:"pageUrl"`
	Streams     []MediaStream `json:"streams"`
	Episodes    []Episode     `json:"episodes"`
	HTMLBytes   int           `json:"htmlBytes"`
	SourceLabel string        `json:"sourceLabel"`
}

type InitialState struct {
	LastOutputDir string `json:"lastOutputDir"`
	FFmpegReady   bool   `json:"ffmpegReady"`
	FFmpegPath    string `json:"ffmpegPath"`
	Platform      string `json:"platform"`
	Version       string `json:"version"`
	BuildDate     string `json:"buildDate"`
	LogDir        string `json:"logDir"`
	LogPath       string `json:"logPath"`
	CanShutdown   bool   `json:"canShutdown"`
}

// SessionEpisode is one row of the saved list, including where it should be
// written and how it ended.
type SessionEpisode struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Number    int           `json:"number"`
	PageURL   string        `json:"pageUrl"`
	StreamURL string        `json:"streamUrl,omitempty"`
	Streams   []MediaStream `json:"streams,omitempty"`
	OutputDir string        `json:"outputDir,omitempty"`
	Selected  bool          `json:"selected"`
	Status    string        `json:"status,omitempty"`
	Message   string        `json:"message,omitempty"`
	Output    string        `json:"output,omitempty"`
}

type SessionMovie struct {
	Key       string           `json:"key"`
	Title     string           `json:"title"`
	Source    string           `json:"source"`
	PageURL   string           `json:"pageUrl"`
	OutputDir string           `json:"outputDir"`
	Collapsed bool             `json:"collapsed"`
	Episodes  []SessionEpisode `json:"episodes"`
}

type SessionState struct {
	Version  string         `json:"version"`
	SavedAt  string         `json:"savedAt"`
	Finished bool           `json:"finished"`
	Movies   []SessionMovie `json:"movies"`
}

type SessionSummary struct {
	Movies    int `json:"movies"`
	Episodes  int `json:"episodes"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Pending   int `json:"pending"`
	// NeedsAttention drives the restore prompt: something failed or never ran.
	NeedsAttention bool   `json:"needsAttention"`
	SavedAt        string `json:"savedAt"`
	Version        string `json:"version"`
}

type SavedSession struct {
	State   SessionState   `json:"state"`
	Summary SessionSummary `json:"summary"`
}

type ShutdownStatus struct {
	Scheduled bool `json:"scheduled"`
	Seconds   int  `json:"seconds"`
	// SurvivesAppExit is false where the countdown is kept by the app itself.
	SurvivesAppExit bool   `json:"survivesAppExit"`
	At              string `json:"at"`
}

type DownloadControlStatus struct {
	Paused bool `json:"paused"`
}

type FFmpegStatus struct {
	Ready bool   `json:"ready"`
	Path  string `json:"path"`
}

type SourceDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type DownloadItem struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Number    int           `json:"number"`
	PageURL   string        `json:"pageUrl"`
	StreamURL string        `json:"streamUrl,omitempty"`
	Streams   []MediaStream `json:"streams,omitempty"`
	Title     string        `json:"title,omitempty"`
	OutputDir string        `json:"outputDir,omitempty"`
}

type DownloadRequest struct {
	Title           string         `json:"title"`
	OutputDir       string         `json:"outputDir"`
	PreferredServer string         `json:"preferredServer"`
	Items           []DownloadItem `json:"items"`
	SkipExisting    bool           `json:"skipExisting"`
}

type QueueEvent struct {
	ID        string `json:"id"`
	Movie     string `json:"movie"`
	Index     int    `json:"index"`
	Total     int    `json:"total"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Message   string `json:"message,omitempty"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
	Attempt   int    `json:"attempt,omitempty"`
}

type ProgressEvent struct {
	ID       string  `json:"id"`
	Index    int     `json:"index"`
	Total    int     `json:"total"`
	Name     string  `json:"name"`
	Time     string  `json:"time"`
	Duration string  `json:"duration,omitempty"`
	Speed    string  `json:"speed"`
	// Percent is -1 while the total duration is unknown.
	Percent float64 `json:"percent"`
}

type DoneEvent struct {
	Total     int  `json:"total"`
	Completed int  `json:"completed"`
	Failed    int  `json:"failed"`
	Skipped   int  `json:"skipped"`
	Cancelled bool `json:"cancelled"`
}
