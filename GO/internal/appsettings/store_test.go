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

func TestSettingsHaravanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "settings.bhconfig"))

	want := Settings{
		Gid:      map[string]string{"MAKH": "1"},
		Zalo:     map[string]string{},
		Reminder: map[string]string{},
		Haravan:  map[string]string{"access_token": "abc123", "exclude_shops": "CLEVY VIỆT NAM"},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load(filepath.Join(dir, "khong-co-settings.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Haravan["access_token"] != "abc123" {
		t.Errorf("access_token = %q, muốn %q", got.Haravan["access_token"], "abc123")
	}
	if got.Haravan["exclude_shops"] != "CLEVY VIỆT NAM" {
		t.Errorf("exclude_shops = %q, muốn %q", got.Haravan["exclude_shops"], "CLEVY VIỆT NAM")
	}
}

func TestLoadFillsEmptyHaravanMap(t *testing.T) {
	// File .bhconfig cũ (viết trước khi có nhánh TMĐT) không có khoá
	// "haravan" — Load phải trả map rỗng chứ không phải nil, để
	// SettingsModal đọc được ngay mà không nil-check ở mọi chỗ dùng.
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	if err := os.WriteFile(path, []byte(`{"gid":{},"zalo":{},"reminder":{}}`), 0o644); err != nil {
		t.Fatalf("ghi file cũ: %v", err)
	}
	got, err := NewStore(path).Load(filepath.Join(dir, "khong-co.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Haravan == nil {
		t.Fatalf("Haravan = nil, muốn map rỗng")
	}
}

func TestStore_Load_FileCũKhôngCóKhốiMisaVẫnRaMapRỗng(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	// Đúng hình dạng file .bhconfig của bản trước khi có MISA.
	if err := os.WriteFile(path, []byte(`{"gid":{"COOP":"1"},"zalo":{},"reminder":{},"haravan":{}}`), 0o644); err != nil {
		t.Fatalf("ghi file cũ: %v", err)
	}

	settings, err := NewStore(path).Load(filepath.Join(dir, "không-có.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Misa == nil {
		t.Error("Misa = nil, want map rỗng — frontend cần object thật để render bảng")
	}
	if settings.MisaRouting == nil {
		t.Error("MisaRouting = nil, want map rỗng")
	}
}

func TestStore_SaveLoad_GiữNguyênKhốiMisa(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(path)

	want := Settings{
		Misa:        map[string]string{"sid_url": "https://script.google.com/x", "db_htla": "Long An"},
		MisaRouting: map[string]string{"Lotte": "htla", "BigC/MT": "ha_thanh"},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(filepath.Join(dir, "không-có.ini"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Misa["sid_url"] != want.Misa["sid_url"] {
		t.Errorf("Misa[sid_url] = %q, want %q", got.Misa["sid_url"], want.Misa["sid_url"])
	}
	if got.MisaRouting["BigC/MT"] != "ha_thanh" {
		t.Errorf("MisaRouting[BigC/MT] = %q, want %q", got.MisaRouting["BigC/MT"], "ha_thanh")
	}
}
