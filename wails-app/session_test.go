package main

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleSession() SessionState {
	return SessionState{Movies: []SessionMovie{{
		Key:       "movie-1",
		Title:     "Ỷ Thiên Đồ Long Ký",
		OutputDir: `D:\Videos`,
		Episodes: []SessionEpisode{
			{ID: "episode-1", Name: "Tập 01", Number: 1, Selected: true, Status: "completed", Output: `D:\Videos\Tap 01.mp4`},
			{ID: "episode-2", Name: "Tập 02", Number: 2, Selected: true, Status: "failed", Message: "HTTP 404"},
			{ID: "episode-3", Name: "Tập 03", Number: 3, Selected: true, OutputDir: `D:\Videos\Tap rieng`},
			{ID: "episode-4", Name: "Tập 04", Number: 4, Selected: false},
			{ID: "episode-5", Name: "Tập 05", Number: 5, Selected: true, Status: "skipped"},
		},
	}}}
}

func TestSessionRoundTripKeepsResultsAndFolders(t *testing.T) {
	store := &sessionStore{path: filepath.Join(t.TempDir(), "state", "session.json")}
	if err := store.save(sampleSession()); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != appVersion || loaded.SavedAt == "" {
		t.Fatalf("saved session must stamp version and time, got %q / %q", loaded.Version, loaded.SavedAt)
	}
	if len(loaded.Movies) != 1 || len(loaded.Movies[0].Episodes) != 5 {
		t.Fatalf("session did not survive the round trip: %+v", loaded.Movies)
	}
	if loaded.Movies[0].Episodes[2].OutputDir != `D:\Videos\Tap rieng` {
		t.Fatal("per-episode folder was lost")
	}
	if loaded.Movies[0].Episodes[1].Message != "HTTP 404" {
		t.Fatal("failure reason was lost")
	}
}

func TestSummarizeSessionCountsEachOutcome(t *testing.T) {
	summary := summarizeSession(sampleSession())
	if summary.Movies != 1 || summary.Episodes != 5 {
		t.Fatalf("movies/episodes = %d/%d, want 1/5", summary.Movies, summary.Episodes)
	}
	if summary.Completed != 1 || summary.Failed != 1 || summary.Skipped != 1 {
		t.Fatalf("completed/failed/skipped = %d/%d/%d, want 1/1/1", summary.Completed, summary.Failed, summary.Skipped)
	}
	// Only the selected episode without a status counts as pending; the
	// unselected one was never queued.
	if summary.Pending != 1 {
		t.Fatalf("pending = %d, want 1", summary.Pending)
	}
	if !summary.NeedsAttention {
		t.Fatal("a session with failures and pending episodes needs the restore prompt")
	}
}

func TestSummarizeFinishedSessionNeedsNoPrompt(t *testing.T) {
	state := SessionState{Movies: []SessionMovie{{Key: "m", Episodes: []SessionEpisode{
		{ID: "e1", Selected: true, Status: "completed"},
		{ID: "e2", Selected: true, Status: "skipped"},
		{ID: "e3", Selected: false},
	}}}}
	if summary := summarizeSession(state); summary.NeedsAttention {
		t.Fatal("a fully downloaded session must not ask to be reopened")
	}
}

func TestLoadSessionToleratesMissingAndBrokenFiles(t *testing.T) {
	directory := t.TempDir()
	store := &sessionStore{path: filepath.Join(directory, "session.json")}
	state, err := store.load()
	if err != nil || len(state.Movies) != 0 {
		t.Fatalf("missing file must load as an empty session, got %+v / %v", state, err)
	}
	if err := os.WriteFile(store.path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err = store.load()
	if err != nil || len(state.Movies) != 0 {
		t.Fatalf("corrupt file must not break startup, got %+v / %v", state, err)
	}
	if err := store.clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.path); !os.IsNotExist(err) {
		t.Fatal("clear must remove the session file")
	}
	if err := store.clear(); err != nil {
		t.Fatal("clearing an already empty session must succeed")
	}
}
