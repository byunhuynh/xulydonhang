package jmart

import "testing"

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// the real (and only available) sample JMart PDF
	// (đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026]
	// [MN_MT_JM0001][05-07-2026][DH01010844].pdf), confirmed during
	// planning by running the actual Go PDF pipeline directly. Every
	// MARKER and its value sit on adjacent, matchable lines here (the
	// product-table divergence is a separate matter, see Task 3) — but
	// Task 6's real golden-fixture run found that the VALUE captured
	// after "Địa chỉ giao hàng:" itself has a real internal defect: line
	// 31 below is verbatim what this repo's own PDF library actually
	// produces for this real sample, "...346Bến..." with ZERO separator
	// at the physical line-wrap point (see addressLineWrapGapPattern's
	// doc comment in extract.go for the full root-cause explanation and
	// citation). ParseOrderInfo repairs it before returning, so
	// wantAddress below is the CORRECTED value, not a literal echo of
	// line 31.
	text := "\n" +
		"ĐC : L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346 Bến Vân Đồn,Phường Vĩnh Hội, TP.Hồ Chí Minh\n" +
		"Đơn vị : HỆ THỐNG SIÊU THỊ JMART\n" +
		"PHIẾU ĐẶT HÀNG NHÀ CUNG CẤP\n" +
		"Tên nhà cung cấp :\n" +
		"Số điện thoại :\n" +
		"Địa chỉ :\n" +
		"CÔNG TY TNHH TMDV XNK HÀTHÀNH\n" +
		"0903 19 11 15\n" +
		"666/46 ĐƯỜNG 3/2.P.14.QUẬN 10,TP.HCM\n" +
		"Ngày in : 05/07/2026\n" +
		"Người in: kimngoc\n" +
		"Số phiếu đặt: DH01010844\n" +
		"Điện thoại:  0707346346 -\n" +
		"Địa chỉ giao hàng:\n" +
		"L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346Bến Vân Đồn, Phường Vĩnh Hội, TP.Hồ Chí Minh\n" +
		"SĐT nhận hàng :\n" +
		"0707346346\n" +
		"\n" +
		"Ghi chú:\n"

	poNumber, entryDate, cancelDate, deliveryAddress, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "DH01010844" {
		t.Errorf("poNumber = %q, want %q", poNumber, "DH01010844")
	}
	if entryDate != "05/07/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "05/07/2026")
	}
	if cancelDate != entryDate {
		t.Errorf("cancelDate = %q, want it to equal entryDate %q (xulydonhang.py:8148, a direct assignment)", cancelDate, entryDate)
	}
	wantAddress := "L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346\nBến Vân Đồn, Phường Vĩnh Hội, TP.Hồ Chí Minh"
	if deliveryAddress != wantAddress {
		t.Errorf("deliveryAddress = %q, want %q", deliveryAddress, wantAddress)
	}
}

func TestParseOrderInfo_AddressWithoutFusionBugPassesThroughUnchanged(t *testing.T) {
	// Inertness check for addressLineWrapGapPattern (extract.go): an
	// address that does NOT have the fusion bug — i.e. already has a
	// real space between a house number and the following capitalized
	// word, no digit-immediately-touching-an-uppercase-letter transition
	// anywhere — must come out of ParseOrderInfo byte-for-byte identical
	// to what was captured, with the repair regex firing zero times.
	// TestParseOrderInfo_ExtractsRealSampleFields above only proves the
	// repair fires correctly on the ONE known-broken real input; nothing
	// previously proved it leaves an already-correct address alone.
	const address = "45 Lê Lợi, Phường Bến Nghé, Quận 1, TP.Hồ Chí Minh"
	text := "Ngày in : 05/07/2026\n" +
		"Số phiếu đặt: DH01010844\n" +
		"Địa chỉ giao hàng:\n" +
		address + "\n" +
		"SĐT nhận hàng :\n" +
		"0707346346\n"

	_, _, _, deliveryAddress, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if deliveryAddress != address {
		t.Errorf("deliveryAddress = %q, want %q unchanged (addressLineWrapGapPattern must be inert on an already-correct address)", deliveryAddress, address)
	}
}

func TestParseOrderInfo_MissingEntryDateMarkerFailsCleanly(t *testing.T) {
	// No "Ngày in" marker at all -> ok=false. Mirrors Python's real
	// crash risk here (xulydonhang.py:8146's .group(1) has no try/except
	// and would raise AttributeError on a None match) with a clean
	// failure instead, per this codebase's established policy.
	text := "Đơn vị : HỆ THỐNG SIÊU THỊ JMART\n" +
		"Số phiếu đặt: DH01010844\n" +
		"Địa chỉ giao hàng:\nSome Address\nSĐT nhận hàng :\n"
	_, _, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no entry-date marker, want false")
	}
}

func TestParseOrderInfo_MissingDeliveryAddressMarkerFailsCleanly(t *testing.T) {
	// No "Địa chỉ giao hàng:...SĐT nhận hàng:" pair -> ok=false. Python's
	// real code has a soft `if m else None` guard here (unlike
	// entry_date/po_number, which crash outright) — but this port gates
	// ok on ALL THREE markers resolving, not just two, since a missing
	// delivery address would otherwise write a literal empty/garbage
	// value into the Excel ShipTo column with no signal anything went
	// wrong (see the plan's own Global Constraints for the full
	// rationale).
	text := "Đơn vị : HỆ THỐNG SIÊU THỊ JMART\n" +
		"Ngày in : 05/07/2026\n" +
		"Số phiếu đặt: DH01010844\n"
	_, _, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no delivery-address markers, want false")
	}
}

func TestExtractProducts_ParsesRealSampleThreeProducts(t *testing.T) {
	// Exact shape of this repo's OWN extractPageTexts output for the
	// product-table region of the real (and only available) sample
	// JMart PDF, confirmed during planning by running the actual Go PDF
	// pipeline. Cross-checked against real Python's own captured output
	// for the SAME file (ran xulydonhang.py's real cat_giua_theo_dong +
	// tachsanpham_JMart directly): Python produced exactly
	// [{Barcode:8936156730886 OUQty:8 TotalPrice:133806.000}
	//  {Barcode:8936156732668 OUQty:12 TotalPrice:26836.000}
	//  {Barcode:8936156732675 OUQty:12 TotalPrice:26836.260}]
	// — the expected values below match this real captured ground
	// truth exactly, re-derived against Go's own (differently-shaped,
	// unsplit) text using the corrected "1.000" anchor (see
	// ExtractProducts's own doc comment for the full explanation of why
	// Python's literal "1.00" anchor cannot be ported as-is).
	text := "Ghi chú:\n" +
		"Thành tiền(Chưa vat)\n" +
		"Chiết khấu\n" +
		"Đơn giá\n" +
		"Số lượng\n" +
		"QC\n" +
		"ĐVT\n" +
		"Tồn kho\n" +
		"Tên đầy đủ\n" +
		"Barcode\n" +
		"Mã vật tư\n" +
		"STT\n" +
		"1,070,448\n" +
		"0\n" +
		"133,806.000\n" +
		"8.000\n" +
		"1.000\n" +
		"Gói\n" +
		"0.000\n" +
		"NƯỚC GIẶT XẢ BLUE ĐẬMĐẶC H. NƯỚC HOA 3.6 L\n" +
		"8936156730886\n" +
		"03021269\n" +
		"1\n" +
		"322,032\n" +
		"0\n" +
		"26,836.000\n" +
		"12.000\n" +
		"1.000\n" +
		"Chai\n" +
		"3.000\n" +
		"NƯỚC LAU BẾP BLUECHANH 560ML\n" +
		"8936156732668\n" +
		"03021252\n" +
		"2\n" +
		"322,035\n" +
		"0\n" +
		"26,836.260\n" +
		"12.000\n" +
		"1.000\n" +
		"Chai\n" +
		"2.000\n" +
		"NƯỚC LAU BẾP BLUEBẠCH TRÀ Ô LIU  560ML\n" +
		"8936156732675\n" +
		"03021257\n" +
		"3\n" +
		"1,714,515\n" +
		"\n" +
		"\n" +
		"\n" +
		"\n" +
		"5.000\n" +
		"Tổng:\n" +
		"1,714,515\n"

	products := ExtractProducts(text)
	if len(products) != 3 {
		t.Fatalf("len(products) = %d, want 3", len(products))
	}
	want := []Product{
		{Barcode: "8936156730886", OUQty: "8", TotalPrice: "133806.000"},
		{Barcode: "8936156732668", OUQty: "12", TotalPrice: "26836.000"},
		{Barcode: "8936156732675", OUQty: "12", TotalPrice: "26836.260"},
	}
	for i, w := range want {
		if products[i] != w {
			t.Errorf("products[%d] = %+v, want %+v", i, products[i], w)
		}
	}
}

func TestExtractProducts_NoStartMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("no start marker anywhere\nTổng:\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoEndMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("Mã vật tư\nSTT\n8936156730886\nno end marker here\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
