package productdata

import "testing"

const testDataPath = "testdata/data.xlsx"

func TestGetCustomerCode_MatchesTrailingDigitsOfColumnC(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Preserves get_makhachhang's bug: it reads column C (index 2), not
	// column B (index 1), for the trailing-digit match — so matching
	// against "999" (column B's store code) must NOT work; matching
	// against the trailing digits of column C ("KH-COOP-001" -> "001")
	// must work instead.
	if got := store.GetCustomerCode("999"); got != "Không tìm thấy" {
		t.Fatalf("GetCustomerCode(999) = %q, want Không tìm thấy (bug preserved: column B is not actually read)", got)
	}
	if got := store.GetCustomerCode("001"); got != "KH-COOP-001" {
		t.Fatalf("GetCustomerCode(001) = %q, want %q", got, "KH-COOP-001")
	}
}

func TestGetSystemForCustomer(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.GetSystemForCustomer("KH-CF-002"); got != "COOPFOOD" {
		t.Fatalf("GetSystemForCustomer(KH-CF-002) = %q, want COOPFOOD", got)
	}
	if got := store.GetSystemForCustomer("no-such-code"); got != "" {
		t.Fatalf("GetSystemForCustomer(no-such-code) = %q, want empty", got)
	}
}

func TestGetCoopfoodAddress(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.GetCoopfoodAddress("KH-CF-002"); got != "12 Nguyễn Huệ" {
		t.Fatalf("GetCoopfoodAddress(KH-CF-002) = %q, want %q", got, "12 Nguyễn Huệ")
	}
}

func TestGetProductInfo(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	info, ok := store.GetProductInfo("SP0001")
	if !ok {
		t.Fatal("GetProductInfo(SP0001) not found")
	}
	if info.Name != "Nước giặt Blue" || info.WeightKg != 3.6 || info.PackSize != 24 {
		t.Fatalf("GetProductInfo(SP0001) = %+v, want Name=Nước giặt Blue WeightKg=3.6 PackSize=24", info)
	}
}

func TestResolveSku_MapsVendorSkuToInternalCode(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.ResolveSku("1234567-1"); got != "SP0001" {
		t.Fatalf("ResolveSku(1234567-1) = %q, want %q", got, "SP0001")
	}
	if got := store.ResolveSku("9999999-9"); got != "9999999" {
		t.Fatalf("ResolveSku(9999999-9) unmapped = %q, want cleaned-but-unmapped %q", got, "9999999")
	}
}

func TestFindSkusMentioned(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	got := store.FindSkusMentioned("Tặng kèm SP0001 khi mua 2")
	if len(got) != 1 || got[0] != "SP0001" {
		t.Fatalf("FindSkusMentioned = %v, want [SP0001]", got)
	}
	if got := store.FindSkusMentioned("không có mã nào ở đây"); len(got) != 0 {
		t.Fatalf("FindSkusMentioned(no match) = %v, want empty", got)
	}
}
