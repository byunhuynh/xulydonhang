// GO/internal/appsettings/migrate_test.go
package appsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIniFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing ini file: %v", err)
	}
	return path
}

func TestMigrateFromOldIni_ParsesAllThreeBlocks(t *testing.T) {
	path := writeIniFile(t, "[GoogleSheets]\n<gid>\nCOOP = 1741405320\n# a comment line\nMAKH = 0\n</gid>\n<zalo>\nMNCOOPMART = Đơn hàng Co-op Miền Nam\n</zalo>\n<reminder>\nMNKINGFOOD = 1\n</reminder>\n")

	settings, migrated, err := migrateFromOldIni(path)
	if err != nil {
		t.Fatalf("migrateFromOldIni returned error: %v", err)
	}
	if !migrated {
		t.Fatal("migrateFromOldIni returned migrated=false for an existing file")
	}
	if settings.Gid["COOP"] != "1741405320" {
		t.Errorf("Gid[COOP] = %q, want %q", settings.Gid["COOP"], "1741405320")
	}
	if settings.Gid["MAKH"] != "0" {
		t.Errorf("Gid[MAKH] = %q, want %q", settings.Gid["MAKH"], "0")
	}
	if len(settings.Gid) != 2 {
		t.Errorf("len(Gid) = %d, want 2 (comment line must be skipped, not parsed as a key)", len(settings.Gid))
	}
	if settings.Zalo["MNCOOPMART"] != "Đơn hàng Co-op Miền Nam" {
		t.Errorf("Zalo[MNCOOPMART] = %q, want %q", settings.Zalo["MNCOOPMART"], "Đơn hàng Co-op Miền Nam")
	}
	if settings.Reminder["MNKINGFOOD"] != "1" {
		t.Errorf("Reminder[MNKINGFOOD] = %q, want %q", settings.Reminder["MNKINGFOOD"], "1")
	}
}

func TestMigrateFromOldIni_FileDoesNotExist(t *testing.T) {
	_, migrated, err := migrateFromOldIni(filepath.Join(t.TempDir(), "does-not-exist.ini"))
	if err != nil {
		t.Fatalf("migrateFromOldIni returned error for a missing file: %v", err)
	}
	if migrated {
		t.Fatal("migrateFromOldIni returned migrated=true for a missing file")
	}
}

func TestMigrateFromOldIni_MissingBlockReturnsEmptyMap(t *testing.T) {
	path := writeIniFile(t, "[GoogleSheets]\n<gid>\nCOOP = 123\n</gid>\n")

	settings, migrated, err := migrateFromOldIni(path)
	if err != nil {
		t.Fatalf("migrateFromOldIni returned error: %v", err)
	}
	if !migrated {
		t.Fatal("migrateFromOldIni returned migrated=false")
	}
	if len(settings.Zalo) != 0 {
		t.Errorf("Zalo = %v, want empty (no <zalo> block in this file)", settings.Zalo)
	}
	if len(settings.Reminder) != 0 {
		t.Errorf("Reminder = %v, want empty (no <reminder> block in this file)", settings.Reminder)
	}
}

func TestParseTagBlock_MalformedLineReturnsError(t *testing.T) {
	path := writeIniFile(t, "<gid>\nCOOP = 123 = 456\n</gid>\n")

	if _, _, err := migrateFromOldIni(path); err == nil {
		t.Fatal("migrateFromOldIni expected error for a <gid> line with more than one '=', got nil")
	}
}
