package main

type MediaStream struct {
	URL    string `json:"url"`
	Kind   string `json:"kind"`
	Server string `json:"server"`
}

type Episode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Number    int    `json:"number"`
	PageURL   string `json:"pageUrl"`
	StreamURL string `json:"streamUrl,omitempty"`
	Current   bool   `json:"current"`
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
	LogDir        string `json:"logDir"`
	LogPath       string `json:"logPath"`
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Number    int    `json:"number"`
	PageURL   string `json:"pageUrl"`
	StreamURL string `json:"streamUrl,omitempty"`
	Title     string `json:"title,omitempty"`
	OutputDir string `json:"outputDir,omitempty"`
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
}

type ProgressEvent struct {
	Index int    `json:"index"`
	Total int    `json:"total"`
	Name  string `json:"name"`
	Time  string `json:"time"`
	Speed string `json:"speed"`
}

type DoneEvent struct {
	Total     int  `json:"total"`
	Completed int  `json:"completed"`
	Failed    int  `json:"failed"`
	Skipped   int  `json:"skipped"`
	Cancelled bool `json:"cancelled"`
}
