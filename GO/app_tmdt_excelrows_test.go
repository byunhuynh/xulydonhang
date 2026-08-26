package main

import (
	"reflect"
	"testing"

	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/tmdt"
)

// tmdtRow dung mot dong TMDTRow toi thieu: Description phai dung khuon
// "TMDT-{kenh} - {shop} - {ma don} - Ngay do {ngay} - {kho}" vi
// shopFromDescription boc ten shop tu do.
func tmdtRow(shop, date, order string) excelwriter.TMDTRow {
	return excelwriter.TMDTRow{
		EntryDate:    date,
		OrderNumber:  "DDHTMDT-Shopee-" + order,
		CustomerCode: "MB_TMDT_00001",
		ShipTo:       "HN",
		SKU:          "TP30022",
		Qty:          1,
		UnitPrice:    1000,
		Note:         order,
		Description:  "TMĐT-Shopee - " + shop + " - " + order + " - Ngày đổ " + date + " - HN",
	}
}

// TestTMDTExcelRowsTheoDungDongCuaNhom khoa lai dieu push MISA dua vao:
// moi dong tom tat TMDT phai mang so dong THAT cua chinh nhom do trong so
// dat hang.
//
// Truoc day app_tmdt.go vut bo startRow tra ve tu WriteTMDTRows (`if _,
// err := ...`), nen moi dong tom tat deu co ExcelRows rong va modal push
// bao "chua co dong nao trong so dat hang" cho toan bo don TMDT.
//
// Cac nhom o day DAN XEN nhau co y: dong Excel cua mot nhom KHONG lien
// tuc, nen phep gan khong the la mot khoang start..end.
func TestTMDTExcelRowsTheoDungDongCuaNhom(t *testing.T) {
	const startRow = 9
	res := tmdt.Result{OrderRows: []excelwriter.TMDTRow{
		tmdtRow("Blue HN", "22/08/2026", "A1"), // dong 9  -> Blue HN
		tmdtRow("Be Clean", "22/08/2026", "B1"), // dong 10 -> Be Clean
		tmdtRow("Blue HN", "22/08/2026", "A2"), // dong 11 -> Blue HN
		tmdtRow("Be Clean", "22/08/2026", "B2"), // dong 12 -> Be Clean
		tmdtRow("Blue HN", "22/08/2026", "A3"), // dong 13 -> Blue HN
	}}

	rows := summaryTMDTRows("XUẤT HÀNG.xlsx", groupTMDTSummary(res, startRow))
	if len(rows) != 2 {
		t.Fatalf("co %d dong tom tat, muon 2", len(rows))
	}

	got := map[string][]int{}
	for _, r := range rows {
		got[r.PO] = r.ExcelRows
	}
	want := map[string][]int{
		"Be Clean · 22/08/2026": {10, 12},
		"Blue HN · 22/08/2026":  {9, 11, 13},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExcelRows =\n  %v\nmuon\n  %v", got, want)
	}
}

func TestTMDTExcelRowsTheoDungStartRow(t *testing.T) {
	// So dat hang da co san du lieu thi startRow khong phai 9 - moi chi so
	// phai dich theo, neu khong push se cat nham dong cua lo truoc.
	res := tmdt.Result{OrderRows: []excelwriter.TMDTRow{
		tmdtRow("Blue HN", "22/08/2026", "A1"),
		tmdtRow("Blue HN", "22/08/2026", "A2"),
	}}

	rows := summaryTMDTRows("f.xlsx", groupTMDTSummary(res, 730))
	if len(rows) != 1 {
		t.Fatalf("co %d dong tom tat, muon 1", len(rows))
	}
	if want := []int{730, 731}; !reflect.DeepEqual(rows[0].ExcelRows, want) {
		t.Fatalf("ExcelRows = %v, muon %v", rows[0].ExcelRows, want)
	}
}
