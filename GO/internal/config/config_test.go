package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetSTT_MissingFileReturnsDefaultOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	store := NewStore(path)

	got, err := store.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if got != 1 {
		t.Fatalf("GetSTT = %d, want 1", got)
	}
}

func TestSetSTTThenGetSTT_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	store := NewStore(path)

	if err := store.SetSTT(108); err != nil {
		t.Fatalf("SetSTT returned error: %v", err)
	}

	got, err := store.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if got != 108 {
		t.Fatalf("GetSTT = %d, want 108", got)
	}
}

func TestGetSTT_InvalidValueReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(path, []byte("current_row=abc\n"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	store := NewStore(path)

	if _, err := store.GetSTT(); err == nil {
		t.Fatal("GetSTT expected error for invalid value, got nil")
	}
}
