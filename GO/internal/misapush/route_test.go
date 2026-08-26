package misapush

import (
	"reflect"
	"sort"
	"testing"
)

func TestRouteKey_JITTáchTheoKho(t *testing.T) {
	if got := RouteKey("JIT-CHOICE", "MN_JIT_01512", "WH6_HN"); got != "JIT-CHOICE/WH6_HN" {
		t.Errorf("RouteKey JIT WH6_HN = %q, want %q", got, "JIT-CHOICE/WH6_HN")
	}
	if got := RouteKey("JIT-CHOICE", "MN_JIT_01512", "WH6_HTLA"); got != "JIT-CHOICE/WH6_HTLA" {
		t.Errorf("RouteKey JIT WH6_HTLA = %q, want %q", got, "JIT-CHOICE/WH6_HTLA")
	}
}

func TestRouteKey_JITThiếuKhoKhôngPanic(t *testing.T) {
	if got := RouteKey("JIT-CHOICE", "MN_JIT_01512", "   "); got != "JIT-CHOICE" {
		t.Errorf("RouteKey JIT không kho = %q, want %q", got, "JIT-CHOICE")
	}
}

func TestRouteKey_BigCTáchTheoPhânKhúc(t *testing.T) {
	// Đúng 4 mã mà bigc.ResolveCustomerCode sinh ra, không hơn.
	cases := map[string]string{
		"MB_GC_BIGC":   "BigC/GC",
		"MN_GC_BIGCAC": "BigC/GC",
		"MB_MT_BIGC":   "BigC/MT",
		"MN_MT_BIGCAC": "BigC/MT",
	}
	for code, want := range cases {
		if got := RouteKey("BigC", code, ""); got != want {
			t.Errorf("RouteKey BigC %s = %q, want %q", code, got, want)
		}
	}
}

func TestRouteKey_BigCMãThiếuPhầnKhôngPanic(t *testing.T) {
	if got := RouteKey("BigC", "BIGCGARDEN", ""); got != "BigC" {
		t.Errorf("RouteKey BigC mã cũ = %q, want %q", got, "BigC")
	}
}

func TestRouteKey_HệThốngKhácGiữNguyên(t *testing.T) {
	if got := RouteKey("Lotte", "MN_MT_LOT1001", ""); got != "Lotte" {
		t.Errorf("RouteKey Lotte = %q, want %q", got, "Lotte")
	}
	if got := RouteKey("TMĐT-Shopee", "MN_TMDT_00015", ""); got != "TMĐT-Shopee" {
		t.Errorf("RouteKey TMĐT = %q, want %q", got, "TMĐT-Shopee")
	}
}

func TestLabel_DễĐọcChoTừngDạngKhoá(t *testing.T) {
	cases := map[string]string{
		"BigC/GC":            "BigC · gia công",
		"BigC/MT":            "BigC · modern trade",
		"BigC":               "BigC",
		"JIT-CHOICE/WH6_HN":  "JIT · kho WH6_HN",
		"JIT-CHOICE":         "JIT",
		"TMĐT-*":             "TMĐT (mọi sàn)",
		"Lotte":              "Lotte",
	}
	for key, want := range cases {
		if got := Label(key); got != want {
			t.Errorf("Label(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestLookup_KhớpĐúngKhôngPhânBiệtHoaThường(t *testing.T) {
	routing := map[string]string{"lotte": BranchHTLA}
	if got := Lookup(routing, "Lotte"); got != BranchHTLA {
		t.Errorf("Lookup Lotte = %q, want %q", got, BranchHTLA)
	}
	if got := Lookup(routing, "Satra"); got != "" {
		t.Errorf("Lookup khoá chưa map = %q, want chuỗi rỗng", got)
	}
}

func TestLookup_TMĐTRơiVềKhoáTiềnTố(t *testing.T) {
	routing := map[string]string{TMDTRouteKey: BranchHTLA}
	if got := Lookup(routing, "TMĐT-Sàn Chưa Từng Thấy"); got != BranchHTLA {
		t.Errorf("Lookup sàn mới = %q, want %q", got, BranchHTLA)
	}
}

func TestLookup_KhớpĐúngThắngTiềnTố(t *testing.T) {
	routing := map[string]string{
		TMDTRouteKey:  BranchHTLA,
		"TMĐT-Shopee": BranchHaThanh,
	}
	if got := Lookup(routing, "TMĐT-Shopee"); got != BranchHaThanh {
		t.Errorf("Lookup TMĐT-Shopee = %q, want %q (khớp đúng phải thắng tiền tố)", got, BranchHaThanh)
	}
}

func TestSeedRouting_PhủMọiHệThốngProcessorSinhRa(t *testing.T) {
	// Danh sách này là MỌI giá trị OrderRow.System mà các processor hiện có
	// sinh ra, cộng hai khoá tách nhỏ. Thêm processor mới mà quên gieo nhánh
	// thì test này đỏ ngay, thay vì lặng lẽ chặn push giữa lúc cần đẩy đơn.
	want := []string{
		"BigC/GC", "BigC/MT",
		"COOPFOOD", "COOPMART", "Coop",
		"Emart", "FujiMart",
		"JIT-CHOICE/WH6_HN", "JIT-CHOICE/WH6_HTLA",
		"JMart", "Kingfood", "Lotte", "MR.DIY", "Satra",
		"TMĐT-*", "Winmart",
	}
	seed := SeedRouting()
	got := make([]string, 0, len(seed))
	for k := range seed {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SeedRouting keys =\n  %v\nwant\n  %v", got, want)
	}
}

func TestSeedRouting_ĐúngNhánhChoTừngKhoá(t *testing.T) {
	seed := SeedRouting()
	htla := []string{"TMĐT-*", "COOPMART", "COOPFOOD", "Coop", "Lotte", "Satra", "MR.DIY", "FujiMart", "BigC/GC", "JIT-CHOICE/WH6_HTLA"}
	haThanh := []string{"BigC/MT", "Emart", "Winmart", "Kingfood", "JMart", "JIT-CHOICE/WH6_HN"}
	for _, k := range htla {
		if seed[k] != BranchHTLA {
			t.Errorf("seed[%q] = %q, want %q", k, seed[k], BranchHTLA)
		}
	}
	for _, k := range haThanh {
		if seed[k] != BranchHaThanh {
			t.Errorf("seed[%q] = %q, want %q", k, seed[k], BranchHaThanh)
		}
	}
}

func TestApplySeed_ChỉThêmKhoáCònThiếu(t *testing.T) {
	// Lotte đã được người dùng đổi sang Hà Thành — bảng gieo KHÔNG được kéo
	// nó về HTLA, nếu không thì mỗi lần sửa hằng số trong code là đẩy đơn
	// sang sổ của pháp nhân khác mà không ai bấm gì.
	routing := map[string]string{"Lotte": BranchHaThanh}

	changed := ApplySeed(routing)

	if !changed {
		t.Error("ApplySeed = false, want true (còn nhiều khoá chưa có)")
	}
	if routing["Lotte"] != BranchHaThanh {
		t.Errorf("ApplySeed ghi đè Lotte thành %q — không được phép", routing["Lotte"])
	}
	if routing["Satra"] != BranchHTLA {
		t.Errorf("ApplySeed không thêm Satra: %q", routing["Satra"])
	}
}

func TestApplySeed_KhôngĐổiGìThìBáoFalse(t *testing.T) {
	routing := SeedRouting()
	if ApplySeed(routing) {
		t.Error("ApplySeed = true trên map đã đủ khoá, want false")
	}
}
