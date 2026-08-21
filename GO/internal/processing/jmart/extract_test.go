package jmart

import "testing"

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// the real (and only available) sample JMart PDF
	// (đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026]
	// [MN_MT_JM0001][05-07-2026][DH01010844].pdf), confirmed during
	// planning by running the actual Go PDF pipeline directly. Unlike
	// most other vendors in this project, this specific region of the
	// PDF (header/PO/date/address) shows NO layout divergence between
	// Go's extraction and PyMuPDF's — both keep every marker and its
	// value on adjacent, unsplit lines here (the divergence in this PDF
	// template is confined to the product table, see Task 3).
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
	wantAddress := "L1 – 01, L1 – 02B Tầng 1, Tòa nhà Gold View, 346Bến Vân Đồn, Phường Vĩnh Hội, TP.Hồ Chí Minh"
	if deliveryAddress != wantAddress {
		t.Errorf("deliveryAddress = %q, want %q", deliveryAddress, wantAddress)
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
