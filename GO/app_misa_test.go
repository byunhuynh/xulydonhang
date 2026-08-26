package main

import (
	"path/filepath"
	"testing"

	"order-processor/internal/appsettings"
	"order-processor/internal/misapush"
)

func newTestAppForMisa(t *testing.T, settings appsettings.Settings) *App {
	t.Helper()
	store := appsettings.NewStore(filepath.Join(t.TempDir(), "settings.bhconfig"))
	if err := store.Save(settings); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	return &App{appSettingsStore: store}
}

func TestMisaResolveRoutes_TáchĐúngBigCVàJIT(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: misapush.SeedRouting()})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{
		{System: "BigC", CustomerCode: "MB_GC_BIGC"},
		{System: "BigC", CustomerCode: "MN_MT_BIGCAC"},
		{System: "JIT-CHOICE", CustomerCode: "MN_JIT_01512", ShipTo: "WH6_HTLA"},
		{System: "JIT-CHOICE", CustomerCode: "MN_JIT_01512", ShipTo: "WH6_HN"},
	})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}

	want := []MisaRouteInfo{
		{Key: "BigC/GC", Label: "BigC · gia công", Branch: misapush.BranchHTLA},
		{Key: "BigC/MT", Label: "BigC · modern trade", Branch: misapush.BranchHaThanh},
		{Key: "JIT-CHOICE/WH6_HTLA", Label: "JIT · kho WH6_HTLA", Branch: misapush.BranchHTLA},
		{Key: "JIT-CHOICE/WH6_HN", Label: "JIT · kho WH6_HN", Branch: misapush.BranchHaThanh},
	}
	if len(got) != len(want) {
		t.Fatalf("số phần tử = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestMisaResolveRoutes_SànTMĐTChưaTừngThấyVẫnRaHTLA(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: misapush.SeedRouting()})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{{System: "TMĐT-Sàn Mới", CustomerCode: "MN_TMDT_00015"}})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}
	if got[0].Branch != misapush.BranchHTLA {
		t.Errorf("Branch = %q, want %q (phải rơi về khoá tiền tố TMĐT-*)", got[0].Branch, misapush.BranchHTLA)
	}
	if got[0].Key != "TMĐT-Sàn Mới" {
		t.Errorf("Key = %q, want %q (khoá giữ nguyên tên sàn để ghi nhớ được)", got[0].Key, "TMĐT-Sàn Mới")
	}
}

func TestMisaResolveRoutes_KhoáChưaMapTrảNhánhRỗng(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: map[string]string{}})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{{System: "Lotte", CustomerCode: "MN_MT_LOT1001"}})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}
	if got[0].Branch != "" {
		t.Errorf("Branch = %q, want chuỗi rỗng — không được đoán bừa một nhánh", got[0].Branch)
	}
}

func TestMisaResolveRoutes_CấuHìnhNgườiDùngThắngBảngGieo(t *testing.T) {
	routing := misapush.SeedRouting()
	routing["Lotte"] = misapush.BranchHaThanh // người dùng đã đổi
	app := newTestAppForMisa(t, appsettings.Settings{MisaRouting: routing})

	got, err := app.MisaResolveRoutes([]MisaRouteInput{{System: "Lotte", CustomerCode: "MN_MT_LOT1001"}})
	if err != nil {
		t.Fatalf("MisaResolveRoutes: %v", err)
	}
	if got[0].Branch != misapush.BranchHaThanh {
		t.Errorf("Branch = %q, want %q", got[0].Branch, misapush.BranchHaThanh)
	}
}

func TestMisaRouteOptions_GộpSeedVàCấuHìnhSắpTheoNhãn(t *testing.T) {
	app := newTestAppForMisa(t, appsettings.Settings{
		MisaRouting: map[string]string{"Lotte": misapush.BranchHaThanh, "SànLạ": misapush.BranchHTLA},
	})

	got, err := app.MisaRouteOptions()
	if err != nil {
		t.Fatalf("MisaRouteOptions: %v", err)
	}

	byKey := map[string]MisaRouteInfo{}
	for _, o := range got {
		byKey[o.Key] = o
	}
	if byKey["Lotte"].Branch != misapush.BranchHaThanh {
		t.Errorf("Lotte = %q, want %q (giá trị đã lưu phải thắng bảng gieo)", byKey["Lotte"].Branch, misapush.BranchHaThanh)
	}
	if _, ok := byKey["SànLạ"]; !ok {
		t.Error("thiếu khoá lạ đã lưu trong cấu hình")
	}
	if byKey["BigC/GC"].Label != "BigC · gia công" {
		t.Errorf("BigC/GC label = %q, want %q", byKey["BigC/GC"].Label, "BigC · gia công")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Label > got[i].Label {
			t.Fatalf("danh sách chưa sắp theo nhãn: %q đứng trước %q", got[i-1].Label, got[i].Label)
		}
	}
}
