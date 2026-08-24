package zalosend

import (
	"errors"
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
