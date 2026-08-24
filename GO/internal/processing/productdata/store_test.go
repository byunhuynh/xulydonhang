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

func TestResolveSkuAliasDoesNotPromoteShortProductToLongerVariant(t *testing.T) {
	store := newStore(nil, [][]string{
		{"SKU", "Name", "Weight", "Pack", "Alias"},
		{"TP31630", "Blue 3.2L", "3.2", "4", "[Top Value] Nước rửa chén Blue túi 3.2L"},
		{"TP31647", "Blue 3.2L không mùi", "3.2", "4", "[Top Value] Nước rửa chén Blue túi 3.2L không mùi"},
	})
	got, ok := store.ResolveSkuAlias("[TopValue]NướcrửachénBluetúi3.2LNew")
	if !ok || got != "TP31630" {
		t.Fatalf("ResolveSkuAlias(short product) = (%q, %v), want (TP31630, true)", got, ok)
	}
}

func TestResolveSkuAliasHandlesTopValueCataloguePresentationWords(t *testing.T) {
	store := newStore(nil, [][]string{
		{"SKU", "Name", "Weight", "Pack", "TopValue"},
		{"TP31333", "Nước xả vải Blue Hương Thanh Xuân túi 2.1L", "2.1", "8", "Nước xả vải Blue Hương Thanh Xuân túi 2.1L"},
	})

	got, ok := store.ResolveSkuAlias("[TopValue]NướcxảvảiBluethanhxuân2,1LNew,2,1l")
	if !ok || got != "TP31333" {
		t.Fatalf("ResolveSkuAlias() = (%q, %v), want (TP31333, true)", got, ok)
	}
}

func TestResolveSku_PreservedBugMapsWeightColumnToo(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Preserves load_sku_mapping's bug: the mapping loop starts at column C
	// (index 2, the weight column) rather than skipping straight to the
	// per-vendor SKU columns further right, so SanPham's weight cell for
	// SP0001 ("3.6" in column C) is itself indexed as a "SKU" that resolves
	// back to SP0001. If the loop's start index were "fixed" to skip
	// non-SKU columns, this would fail.
	if got := store.ResolveSku("3.6"); got != "SP0001" {
		t.Fatalf("ResolveSku(3.6) = %q, want %q (bug preserved: weight column C is mapped too)", got, "SP0001")
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

func TestGetCustomerCodeByFuzzyAddress_MatchesSatraByAddress(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	// Exact match (after normalization) must resolve with a high score.
	got, ok := store.GetCustomerCodeByFuzzyAddress("SATRA", "123 Nguyễn Huệ, Phường Bến Nghé, Quận 1, Tp.HCM, VNM")
	if !ok || got != "MN_MT_TESTSTF" {
		t.Fatalf("GetCustomerCodeByFuzzyAddress(SATRA, exact) = (%q, %v), want (%q, true)", got, ok, "MN_MT_TESTSTF")
	}
	// A wildly different address must not match (score well under the 95 threshold).
	if _, ok := store.GetCustomerCodeByFuzzyAddress("SATRA", "999 Đường Không Tồn Tại, Xã Lạ, Tỉnh Khác"); ok {
		t.Fatal("GetCustomerCodeByFuzzyAddress(SATRA, unrelated) = matched, want no match")
	}
	// System filter: querying under a system that has no rows must not match,
	// even with the exact same address text.
	if _, ok := store.GetCustomerCodeByFuzzyAddress("BIGC", "123 Nguyễn Huệ, Phường Bến Nghé, Quận 1, Tp.HCM, VNM"); ok {
		t.Fatal("GetCustomerCodeByFuzzyAddress(BIGC, exact SATRA address) = matched, want no match (system filter)")
	}
}

func TestGetCustomerCodeByFuzzyAddress_BlankColumnANeverMatches(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	// The fixture's blank-column-A row (colC="BLANK_A_TESTCODE") has a
	// populated column D that would otherwise fuzzy-match its own address
	// text perfectly. Mirrors Python's `if col_A and ...` truthiness guard
	// (laymakhachhang_satra, xulydonhang.py:278): a row whose column A is
	// blank must never match, for ANY system queried — since
	// strings.Contains(x, "") is always true in Go, this guard must be
	// explicit, not incidental.
	address := "456 Le Loi, Phuong Ben Thanh, Quan 1, Tp.HCM, VNM"
	for _, system := range []string{"SATRA", "BIGC", "COOP", ""} {
		if got, ok := store.GetCustomerCodeByFuzzyAddress(system, address); ok {
			t.Fatalf("GetCustomerCodeByFuzzyAddress(%q, blank-column-A address) = (%q, true), want no match", system, got)
		}
	}
}

func TestGetCustomerCodeBySuffix_MatchesLotteBySystemAndSuffix(t *testing.T) {
	store, err := Load(testDataPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := store.GetCustomerCodeBySuffix("LOTTE", "003"); got != "KH-LOTTE-003" {
		t.Fatalf("GetCustomerCodeBySuffix(LOTTE, 003) = %q, want %q", got, "KH-LOTTE-003")
	}
	// "001" is a real suffix of the COOP row's column C ("KH-COOP-001")
	// — querying it under system "LOTTE" must NOT cross over and match
	// that row; the system filter must be applied first.
	if got := store.GetCustomerCodeBySuffix("LOTTE", "001"); got != "" {
		t.Fatalf("GetCustomerCodeBySuffix(LOTTE, 001) = %q, want empty (system filter must exclude the COOP row)", got)
	}
	if got := store.GetCustomerCodeBySuffix("LOTTE", "999"); got != "" {
		t.Fatalf("GetCustomerCodeBySuffix(LOTTE, 999) = %q, want empty (no matching suffix)", got)
	}
}

// TestGetSiteValue uses synthetic customerRows (via newStore) rather than
// the shared testDataPath fixture, mirroring real MaKH data confirmed live
// (BIGC rows "GO! AN LAC" -> "BIGCANLAC", "GO! VINH" -> "BIGCVINH", and
// "GO! VINH PHUC" -> "BIGCVP" — the last pair is the real prefix-collision
// case GetSiteValue's longest-match rule exists for).
func TestGetSiteValue(t *testing.T) {
	customerRows := [][]string{
		{"header row, skipped"},
		{"BIGC", "GO! AN LAC", "BIGCANLAC", ""},
		{"BIGC", "GO! VINH", "BIGCVINH", ""},
		{"BIGC", "GO! VINH PHUC", "BIGCVP", ""},
	}
	store := newStore(customerRows, nil)

	if got := store.GetSiteValue("GO! AN LAC"); got != "BIGCANLAC" {
		t.Fatalf("GetSiteValue(clean exact match) = %q, want BIGCANLAC", got)
	}

	// Real captured shape (2632058001987.pdf page 2): this port's own PDF
	// extraction glues the store-name line directly onto its own address
	// line with no separator, unlike PyMuPDF's clean split — GetSiteValue
	// must still recover the right site value from that glued text.
	if got := store.GetSiteValue("GO! AN LACSO 1231 KP 5, DUONG QUOC LO 1A"); got != "BIGCANLAC" {
		t.Fatalf("GetSiteValue(glued store name + address) = %q, want BIGCANLAC", got)
	}

	// "GO! VINH" is a genuine prefix of "GO! VINH PHUC" — glued text for
	// the Vinh Phuc store must resolve to Vinh Phuc's OWN site value, not
	// fall through to the shorter Vinh match.
	if got := store.GetSiteValue("GO! VINH PHUCKM6+600, DUONG QUOC LO 2"); got != "BIGCVP" {
		t.Fatalf("GetSiteValue(glued Vinh Phuc) = %q, want BIGCVP (longest prefix, not GO! VINH's BIGCVINH)", got)
	}

	// No exact or prefix match at all: falls back to Python's own
	// `congtrinh.replace(" ", "")` — every space removed, not trimmed.
	if got := store.GetSiteValue("Unknown Store Name"); got != "UnknownStoreName" {
		t.Fatalf("GetSiteValue(no match) = %q, want UnknownStoreName", got)
	}
}
