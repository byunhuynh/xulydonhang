package main

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir doi thu muc lam viec trong pham vi mot test roi tra lai.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestResolveRepoDir_TimThayThiDungThuMucChuaFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.ini"), []byte("x"), 0o600); err != nil {
		t.Fatalf("ghi settings.ini: %v", err)
	}
	chdir(t, dir)

	got, err := filepath.EvalSymlinks(resolveRepoDir("settings.ini"))
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Errorf("resolveRepoDir = %q, want %q", got, want)
	}
}

func TestResolveRepoDir_KhongTimThayThiDungThuMucChuaExe(t *testing.T) {
	// Thu muc trien khai that (New folder tren Desktop) KHONG co
	// settings.ini. Truoc day nhanh nay tra ve "." nen moi duong dan
	// tinh theo THU MUC LAM VIEC: bam dup vao exe thi dung, nhung chay
	// qua shortcut co "Start in" khac, hoac tu cua so lenh o thu muc
	// khac, la app doc cau hinh rong ma khong bao gi.
	chdir(t, t.TempDir()) // CWD khong co settings.ini, cha cung khong

	got := resolveRepoDir("settings.ini")
	if got == "." {
		t.Fatal(`resolveRepoDir = "." - van bam theo CWD, thu muc trien khai chua di chuyen duoc`)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("khong lay duoc duong dan exe: %v", err)
	}
	want := filepath.Dir(exe)
	if got != want {
		t.Errorf("resolveRepoDir = %q, want %q (thu muc chua exe)", got, want)
	}
}

func TestResolveRepoFile_KhongTimThayThiTraDuongDanCanhExe(t *testing.T) {
	chdir(t, t.TempDir())

	got := resolveRepoFile("khong-he-ton-tai.xlsx")
	if !filepath.IsAbs(got) {
		t.Errorf("resolveRepoFile = %q, want duong dan tuyet doi canh exe", got)
	}
	if filepath.Base(got) != "khong-he-ton-tai.xlsx" {
		t.Errorf("resolveRepoFile = %q, mat ten file goc", got)
	}
}
