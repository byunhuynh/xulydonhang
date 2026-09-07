package zalosend

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveContact_Found(t *testing.T) {
	zaloMap := map[string]string{"MNBIGC": "Đơn hàng Siêu thị Big-C MN"}
	got, err := ResolveContact("BigC", "MN00123", zaloMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Đơn hàng Siêu thị Big-C MN" {
		t.Fatalf("got %q, want %q", got, "Đơn hàng Siêu thị Big-C MN")
	}
}

func TestResolveContact_RegionPrefixIsCaseInsensitive(t *testing.T) {
	zaloMap := map[string]string{"MBWINMART": "ĐƠN HÀNG WINMART MIỀN BẮC"}
	got, err := ResolveContact("Winmart", "mb00987", zaloMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ĐƠN HÀNG WINMART MIỀN BẮC" {
		t.Fatalf("got %q, want %q", got, "ĐƠN HÀNG WINMART MIỀN BẮC")
	}
}

func TestResolveContact_CoopMapsToCoopmartKeyNotCoop(t *testing.T) {
	zaloMap := map[string]string{"MNCOOPMART": "Đơn hàng Co-op Miền Nam"}
	got, err := ResolveContact("Coop", "MN00123", zaloMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Đơn hàng Co-op Miền Nam" {
		t.Fatalf("got %q, want %q", got, "Đơn hàng Co-op Miền Nam")
	}
}

func TestResolveContact_NotConfigured(t *testing.T) {
	_, err := ResolveContact("Satra", "MN00123", map[string]string{"MNCOOPMART": "x"})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}

func TestResolveContact_EmptyValueTreatedAsNotConfigured(t *testing.T) {
	_, err := ResolveContact("BigC", "MN00123", map[string]string{"MNBIGC": ""})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}

// A customer-code lookup that failed upstream (see satra_processor.go's
// "Không xác định" / coop_processor.go's "Không tìm thấy" fallbacks) has
// no real region prefix - this must fail cleanly via ErrNoContact, not
// panic on a multi-byte UTF-8 rune slice.
func TestResolveContact_NonRegionCustomerCodeFallsBackCleanly(t *testing.T) {
	_, err := ResolveContact("Satra", "Không xác định", map[string]string{"MNSATRA": "x"})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}

func TestResolveContact_ShortCustomerCodeFallsBackCleanly(t *testing.T) {
	_, err := ResolveContact("Satra", "M", map[string]string{"MNSATRA": "x"})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}

func TestResolveContact_EmptyCustomerCodeFallsBackCleanly(t *testing.T) {
	_, err := ResolveContact("Satra", "", map[string]string{"MNSATRA": "x"})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}

// Mã khách hàng có dạng <miền>_<phân khúc>_<mã NCC>: "MB_GC_bgc06" là
// BigC Gia Công miền Bắc, "MB_MT_bgc06" là BigC Modern Trade miền Bắc.
// Hai loại đơn này đi về HAI group Zalo khác nhau, nên key cụ thể
// (miền+phân khúc+hệ thống) phải được thử TRƯỚC key chung.
func TestResolveContact_SegmentKeyBeatsPlainRegionKey(t *testing.T) {
	zaloMap := map[string]string{
		"MBBIGC":   "Đơn hàng BigC - miền Bắc",
		"MBGCBigC": "OEM- CRV size 10l",
	}

	gc, err := ResolveContact("BigC", "MB_GC_bgc06", zaloMap)
	if err != nil {
		t.Fatalf("đơn Gia Công: %v", err)
	}
	if gc != "OEM- CRV size 10l" {
		t.Fatalf("đơn Gia Công gửi tới %q, want group Gia Công", gc)
	}

	mt, err := ResolveContact("BigC", "MB_MT_bgc06", zaloMap)
	if err != nil {
		t.Fatalf("đơn Modern Trade: %v", err)
	}
	if mt != "Đơn hàng BigC - miền Bắc" {
		t.Fatalf("đơn Modern Trade gửi tới %q, want group BigC miền Bắc", mt)
	}
}

// Không cấu hình key phân khúc thì mọi thứ chạy y như trước - đây là
// điều kiện để thay đổi này không đụng tới 9 hệ thống còn lại.
func TestResolveContact_FallsBackToRegionKeyWhenNoSegmentKey(t *testing.T) {
	zaloMap := map[string]string{"MNCOOPMART": "Đơn hàng Co-op Miền Nam"}

	got, err := ResolveContact("Coop", "MN_MT_cop120", zaloMap)
	if err != nil {
		t.Fatalf("ResolveContact: %v", err)
	}
	if got != "Đơn hàng Co-op Miền Nam" {
		t.Fatalf("ResolveContact = %q, want key chung", got)
	}
}

// Key trong Cài đặt do người dùng gõ tay ("MBGCBigC", không phải
// "MBGCBIGC"), nên tra cứu phải bỏ qua hoa/thường.
func TestResolveContact_SegmentKeyIgnoresLetterCase(t *testing.T) {
	got, err := ResolveContact("BigC", "mb_gc_bgc06", map[string]string{"mbgcbigc": "OEM"})
	if err != nil {
		t.Fatalf("ResolveContact: %v", err)
	}
	if got != "OEM" {
		t.Fatalf("ResolveContact = %q, want OEM", got)
	}
}

// Lỗi phải nói ra CẢ HAI key đã thử, nếu không người dùng không biết gõ
// gì vào Cài đặt > Zalo.
func TestResolveContact_ErrorNamesBothKeysTried(t *testing.T) {
	_, err := ResolveContact("BigC", "MB_GC_bgc06", map[string]string{})
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"MBGCBIGC", "MBBIGC"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("lỗi %q không nhắc tới key %q", err.Error(), want)
		}
	}
}

// Đơn Top Value dùng key ĐÚNG NHƯ System của nó: "MNJIT-CHOICE". Không
// có ánh xạ ẩn "JIT-CHOICE"->"JIT" trong code; key cũ "MNJIT" phải đổi
// tên trong Cài đặt > Zalo (quyết định của người dùng, 25/08/2026).
func TestResolveContact_JITChoiceUsesItsOwnKeyVerbatim(t *testing.T) {
	got, err := ResolveContact("JIT-CHOICE", "MN_TV_htla", map[string]string{"MNJIT-CHOICE": "[MII_WH6] Hà Thành"})
	if err != nil {
		t.Fatalf("ResolveContact: %v", err)
	}
	if got != "[MII_WH6] Hà Thành" {
		t.Fatalf("ResolveContact = %q, want group JIT", got)
	}

	if _, err := ResolveContact("JIT-CHOICE", "MN_TV_htla", map[string]string{"MNJIT": "x"}); err == nil {
		t.Fatal("key cũ MNJIT không được khớp nữa - phải báo lỗi để người dùng biết mà đổi tên")
	}
}

// ---- ResolveTarget: cái mà bản xem trước tin nhắn hiển thị ----
//
// ResolveTarget trả về ĐÚNG nhóm mà ResolveContact sẽ gửi tới, kèm key
// đã khớp — bản xem trước và bản gửi thật buộc phải cùng một nguồn, nên
// mỗi test ở đây còn khẳng định lại kết quả khớp với ResolveContact.

func TestResolveTarget_ReturnsMatchedKeyAndGroupName(t *testing.T) {
	zaloMap := map[string]string{"MNBIGC": "Đơn hàng Siêu thị Big-C MN"}
	got := ResolveTarget("BigC", "MN00123", zaloMap)

	if got.Key != "MNBIGC" {
		t.Errorf("Key = %q, want %q", got.Key, "MNBIGC")
	}
	if got.GroupName != "Đơn hàng Siêu thị Big-C MN" {
		t.Errorf("GroupName = %q, want the configured group", got.GroupName)
	}
	if !got.Configured() {
		t.Error("Configured() = false, want true")
	}

	contact, err := ResolveContact("BigC", "MN00123", zaloMap)
	if err != nil || contact != got.GroupName {
		t.Errorf("ResolveContact = (%q, %v), want it to agree with ResolveTarget's %q", contact, err, got.GroupName)
	}
}

// Chưa gán nhóm là trường hợp bản xem trước phải nói được NHIỀU nhất:
// người dùng cần biết chính xác key nào để thêm vào Cài đặt > Zalo.
func TestResolveTarget_UnconfiguredReportsEveryCandidateKey(t *testing.T) {
	got := ResolveTarget("BigC", "MB_GC_bgc06", map[string]string{})

	if got.Configured() {
		t.Fatal("Configured() = true, want false for an empty settings map")
	}
	if got.Key != "" {
		t.Errorf("Key = %q, want empty when nothing matched", got.Key)
	}
	want := []string{"MBGCBIGC", "MBBIGC"}
	if len(got.CandidateKeys) != len(want) {
		t.Fatalf("CandidateKeys = %v, want %v", got.CandidateKeys, want)
	}
	for i, key := range want {
		if got.CandidateKeys[i] != key {
			t.Errorf("CandidateKeys[%d] = %q, want %q", i, got.CandidateKeys[i], key)
		}
	}
	// Key gán mặc định là key CHUNG (không phân khúc) — key phân khúc chỉ
	// dùng khi người dùng chủ động muốn tách đích đến.
	if got.SuggestedKey() != "MBBIGC" {
		t.Errorf("SuggestedKey() = %q, want the plain region key %q", got.SuggestedKey(), "MBBIGC")
	}
}

func TestResolveTarget_SegmentKeyBeatsPlainRegionKey(t *testing.T) {
	zaloMap := map[string]string{
		"MBGCBIGC": "BigC Gia Công MB",
		"MBBIGC":   "BigC chung MB",
	}
	got := ResolveTarget("BigC", "MB_GC_bgc06", zaloMap)

	if got.Key != "MBGCBIGC" || got.GroupName != "BigC Gia Công MB" {
		t.Errorf("ResolveTarget = {%q, %q}, want the segment key to win", got.Key, got.GroupName)
	}
	contact, _ := ResolveContact("BigC", "MB_GC_bgc06", zaloMap)
	if contact != got.GroupName {
		t.Errorf("ResolveContact = %q, want it to agree with ResolveTarget", contact)
	}
}

// Key trong Cài đặt do người dùng gõ tay nên hoa/thường không đồng nhất.
// Bản xem trước phải hiện key ĐÚNG NHƯ TRONG CÀI ĐẶT, không phải bản
// viết hoa app tự dựng — người dùng đi tìm dòng đó để sửa.
func TestResolveTarget_KeyIsTheOneSpelledInSettings(t *testing.T) {
	got := ResolveTarget("BigC", "MB_GC_bgc06", map[string]string{"MBGCBigC": "BigC Gia Công MB"})

	if got.Key != "MBGCBigC" {
		t.Errorf("Key = %q, want the settings' own spelling %q", got.Key, "MBGCBigC")
	}
	if got.GroupName != "BigC Gia Công MB" {
		t.Errorf("GroupName = %q, want the configured group", got.GroupName)
	}
}

func TestResolveTarget_CoopUsesCoopmartKey(t *testing.T) {
	got := ResolveTarget("Coop", "MN_MT_cop120", map[string]string{"MNCOOPMART": "Coopmart MN"})
	if got.Key != "MNCOOPMART" || got.GroupName != "Coopmart MN" {
		t.Errorf("ResolveTarget = {%q, %q}, want the COOPMART override", got.Key, got.GroupName)
	}
}

func TestResolveTarget_EmptyValueIsNotConfigured(t *testing.T) {
	got := ResolveTarget("Satra", "MN00123", map[string]string{"MNSATRA": ""})
	if got.Configured() {
		t.Errorf("ResolveTarget = %+v, want an empty value to count as unconfigured", got)
	}
	if got.SuggestedKey() != "MNSATRA" {
		t.Errorf("SuggestedKey() = %q, want %q", got.SuggestedKey(), "MNSATRA")
	}
}
