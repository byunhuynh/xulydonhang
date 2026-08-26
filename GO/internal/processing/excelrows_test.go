package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing/productdata"
)

func TestExcelRowsFrom(t *testing.T) {
	got := excelRowsFrom(9, 3)
	want := []int{9, 10, 11}
	if len(got) != len(want) {
		t.Fatalf("excelRowsFrom(9,3) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("excelRowsFrom(9,3) = %v, want %v", got, want)
		}
	}
	if r := excelRowsFrom(9, 0); len(r) != 0 {
		t.Errorf("excelRowsFrom(9,0) = %v, want rong", r)
	}
}

// TestCoopExcelRowsTroDungDongDaGhi khoa lai dieu ma push MISA dua vao:
// OrderRow.ExcelRows phai la so dong TUYET DOI trong so dat hang, tro dung
// vao nhung dong cua chinh don do.
//
// Truoc day chi BigC va JIT dien truong nay; Coop va 6 vendor khac de
// trong, nen modal push bo qua het va bao "chua co dong nao trong so dat
// hang" - khong day duoc don Coop nao.
func TestCoopExcelRowsTroDungDongDaGhi(t *testing.T) {
	fixturePaths, _ := filepath.Glob("coop/testdata/fixtures/*.json")
	var real []string
	for _, p := range fixturePaths {
		if filepath.Base(p) != "_frozen_pricing.json" {
			real = append(real, p)
		}
	}
	if len(real) == 0 {
		t.Skip("khong co fixture")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("nap data.xlsx: %v", err)
	}
	pricingSource := loadFrozenPricingSource(t)

	raw, err := os.ReadFile(real[0])
	if err != nil {
		t.Fatalf("doc fixture: %v", err)
	}
	var fixture fixtureData
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
	copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)
	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}

	rows, err := rp.Process(context.Background(), filepath.Join("coop", "testdata", "realpdfs", fixture.SourcePDF))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("Process khong tra dong nao")
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("mo workbook: %v", err)
	}
	defer f.Close()

	for i, row := range rows {
		if row.StatusKind == StatusKindFailed {
			continue
		}
		if len(row.ExcelRows) == 0 {
			t.Fatalf("don[%d] (PO %s): ExcelRows rong - push MISA se bo qua don nay", i, row.PO)
		}
		for _, r := range row.ExcelRows {
			if r < 9 {
				t.Errorf("don[%d]: so dong %d < 9 (vung du lieu bat dau tu dong 9)", i, r)
				continue
			}
			got, err := f.GetCellValue("Don dat hang", fmt.Sprintf("B%d", r))
			if err != nil {
				t.Fatalf("doc B%d: %v", r, err)
			}
			// Cot B mang so don CO TIEN TO (vd "DDHCOOP-102945235-00"),
			// nen kiem CHUA chu khong kiem bang.
			if !strings.Contains(got, row.PO) {
				t.Errorf("don[%d] (PO %s): dong %d mang so don %q - ExcelRows tro nham dong", i, row.PO, r, got)
			}
		}
	}
}
