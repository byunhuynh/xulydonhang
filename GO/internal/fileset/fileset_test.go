package fileset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilterValid_KeepsOnlyAllowedExtensions(t *testing.T) {
	input := []string{"a.pdf", "b.xlsx", "c.txt", "d.docx", "e.PDF"}
	got := FilterValid(input)
	want := []string{"a.pdf", "b.xlsx", "c.txt", "e.PDF"}

	if len(got) != len(want) {
		t.Fatalf("FilterValid(%v) = %v, want %v", input, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FilterValid(%v) = %v, want %v", input, got, want)
		}
	}
}

func TestListFiles_ReturnsOnlyAllowedFilesNotDirs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "order1.pdf"))
	mustWrite(t, filepath.Join(dir, "order2.xlsx"))
	mustWrite(t, filepath.Join(dir, "notes.docx"))
	if err := os.Mkdir(filepath.Join(dir, "08-2026"), 0o755); err != nil {
		t.Fatalf("setup mkdir failed: %v", err)
	}

	got, err := ListFiles(dir)
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListFiles returned %d files, want 2: %v", len(got), got)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed writing %q: %v", path, err)
	}
}
