package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSettingsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing settings file: %v", err)
	}
	return path
}

func TestLoadGidMap_ParsesGidBlock(t *testing.T) {
	path := writeSettingsFile(t, "[GoogleSheets]\n<gid>\nCOOP = 1741405320\nBIGC = 925001622\n</gid>\n")

	gidMap, err := LoadGidMap(path)
	if err != nil {
		t.Fatalf("LoadGidMap returned error: %v", err)
	}
	if gidMap["COOP"] != "1741405320" {
		t.Fatalf("gidMap[COOP] = %q, want %q", gidMap["COOP"], "1741405320")
	}
	if gidMap["BIGC"] != "925001622" {
		t.Fatalf("gidMap[BIGC] = %q, want %q", gidMap["BIGC"], "925001622")
	}
}

func TestLoadGidMap_NoGidBlockReturnsEmptyMap(t *testing.T) {
	path := writeSettingsFile(t, "[GoogleSheets]\nCOOP = 1741405320\n")

	gidMap, err := LoadGidMap(path)
	if err != nil {
		t.Fatalf("LoadGidMap returned error: %v", err)
	}
	if len(gidMap) != 0 {
		t.Fatalf("gidMap = %v, want empty", gidMap)
	}
}

func TestLoadGidMap_MalformedLineReturnsError(t *testing.T) {
	path := writeSettingsFile(t, "<gid>\nCOOP = 123 = 456\n</gid>\n")

	if _, err := LoadGidMap(path); err == nil {
		t.Fatal("LoadGidMap expected error for a line with more than one '=', got nil")
	}
}
