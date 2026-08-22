// GO/internal/appsettings/store_test.go
package appsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_Load_MigratesFromOldIni(t *testing.T) {
	dir := t.TempDir()
	oldIniPath := filepath.Join(dir, "settings.ini")
	if err := os.WriteFile(oldIniPath, []byte("<gid>\nCOOP = 1741405320\n</gid>\n<zalo>\nMNCOOPMART = Đơn hàng Co-op Miền Nam\n</zalo>\n<reminder>\nMNKINGFOOD = 1\n</reminder>\n"), 0o644); err != nil {
		t.Fatalf("failed writing old ini file: %v", err)
	}
	newPath := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(newPath)

	settings, err := store.Load(oldIniPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if settings.Gid["COOP"] != "1741405320" {
		t.Errorf("Gid[COOP] = %q, want %q", settings.Gid["COOP"], "1741405320")
	}
	if settings.Zalo["MNCOOPMART"] != "Đơn hàng Co-op Miền Nam" {
		t.Errorf("Zalo[MNCOOPMART] = %q, want %q", settings.Zalo["MNCOOPMART"], "Đơn hàng Co-op Miền Nam")
	}

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("settings.bhconfig was not created on disk: %v", err)
	}
	if _, err := os.Stat(oldIniPath); err != nil {
		t.Errorf("old settings.ini was removed or is inaccessible — must be left untouched: %v", err)
	}
}

func TestStore_Load_PrefersNewFileOverOldIni(t *testing.T) {
	dir := t.TempDir()
	oldIniPath := filepath.Join(dir, "settings.ini")
	if err := os.WriteFile(oldIniPath, []byte("<gid>\nCOOP = old-value\n</gid>\n"), 0o644); err != nil {
		t.Fatalf("failed writing old ini file: %v", err)
	}
	newPath := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(newPath)
	if err := store.Save(Settings{Gid: map[string]string{"COOP": "new-value"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	settings, err := store.Load(oldIniPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if settings.Gid["COOP"] != "new-value" {
		t.Errorf("Gid[COOP] = %q, want %q (Load must prefer the .bhconfig file over settings.ini once it exists)", settings.Gid["COOP"], "new-value")
	}
}

func TestStore_Load_NeitherFileExists(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "settings.bhconfig"))

	settings, err := store.Load(filepath.Join(dir, "settings.ini"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if settings.Gid == nil || len(settings.Gid) != 0 {
		t.Errorf("Gid = %v, want empty non-nil map", settings.Gid)
	}
	if settings.Zalo == nil || len(settings.Zalo) != 0 {
		t.Errorf("Zalo = %v, want empty non-nil map", settings.Zalo)
	}
	if settings.Reminder == nil || len(settings.Reminder) != 0 {
		t.Errorf("Reminder = %v, want empty non-nil map", settings.Reminder)
	}
}

func TestStore_Save_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(path)
	want := Settings{
		Gid:      map[string]string{"COOP": "123", "BIGC": "456"},
		Zalo:     map[string]string{"MNCOOPMART": "Đơn hàng Co-op Miền Nam"},
		Reminder: map[string]string{"MNKINGFOOD": "1"},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	got, err := store.Load(filepath.Join(dir, "settings.ini"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.Gid["COOP"] != "123" || got.Gid["BIGC"] != "456" {
		t.Errorf("Gid = %v, want %v", got.Gid, want.Gid)
	}
	if got.Zalo["MNCOOPMART"] != "Đơn hàng Co-op Miền Nam" {
		t.Errorf("Zalo = %v, want %v", got.Zalo, want.Zalo)
	}
	if got.Reminder["MNKINGFOOD"] != "1" {
		t.Errorf("Reminder = %v, want %v", got.Reminder, want.Reminder)
	}
}
