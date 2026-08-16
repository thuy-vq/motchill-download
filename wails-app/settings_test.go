package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRememberOutputDirectoryMoreThanOnce(t *testing.T) {
	store := &settingsStore{path: filepath.Join(t.TempDir(), "settings.json")}
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := store.setOutputDir(first); err != nil {
		t.Fatal(err)
	}
	if err := store.setOutputDir(second); err != nil {
		t.Fatal(err)
	}
	reloaded := &settingsStore{path: store.path}
	reloaded.load()
	if got := reloaded.snapshot().LastOutputDir; got != second {
		t.Fatalf("expected %q, got %q", second, got)
	}
}

func TestVideoFingerprintDetectsDuplicateOutput(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "episode-1.mp4")
	duplicate := filepath.Join(directory, "episode-2.mp4")
	different := filepath.Join(directory, "episode-3.mp4")
	if err := os.WriteFile(first, []byte("video-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(duplicate, []byte("video-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(different, []byte("video-three"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstHash, err := videoFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	duplicateHash, err := videoFingerprint(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	differentHash, err := videoFingerprint(different)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != duplicateHash {
		t.Fatal("identical files must have the same fingerprint")
	}
	if firstHash == differentHash {
		t.Fatal("different files must not have the same fingerprint")
	}
}

func TestFolderForRevealUsesTheActualOutputDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "Thu muc co khoang trang")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(directory, "Phim - Tap 01.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolved, err := folderForReveal(video)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != directory {
		t.Fatalf("expected %q, got %q", directory, resolved)
	}

	missingVideo := filepath.Join(directory, "Phim - Tap 02.mp4")
	resolved, err = folderForReveal(missingVideo)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != directory {
		t.Fatalf("expected parent of missing output %q, got %q", directory, resolved)
	}
}
