package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"order-processor/internal/misapush"
	"order-processor/internal/processing/productdata"
)

// TestMoiDonThanhCongDeuCoExcelRows ra soat TOAN BO vendor tren fixture
// that: bat ky dong nao KHONG that bai ma thieu ExcelRows deu la mot don
// push MISA se bo qua im lang.
func TestMoiDonThanhCongDeuCoExcelRows(t *testing.T) {
	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("nap data.xlsx: %v", err)
	}

	vendors := []struct {
		name    string
		pricing func(*testing.T) PricingSource
	}{
		{"coop", func(t *testing.T) PricingSource { return loadFrozenPricingSource(t) }},
		{"bigc", func(t *testing.T) PricingSource { return loadFrozenBigcPricingSource(t) }},
		{"emart", func(t *testing.T) PricingSource { return loadFrozenEmartPricingSource(t) }},
		{"fujimart", func(t *testing.T) PricingSource { return loadFrozenFujimartPricingSource(t) }},
		{"jmart", func(t *testing.T) PricingSource { return loadFrozenJMartPricingSource(t) }},
		{"kingfood", func(t *testing.T) PricingSource { return loadFrozenKingfoodPricingSource(t) }},
		{"lotte", func(t *testing.T) PricingSource { return loadFrozenLottePricingSource(t) }},
		{"satra", func(t *testing.T) PricingSource { return loadFrozenSatraPricingSource(t) }},
		{"winmart", func(t *testing.T) PricingSource { return loadFrozenWinmartPricingSource(t) }},
	}

	noRoute := map[string]int{}
	for _, v := range vendors {
		paths, _ := filepath.Glob(filepath.Join(v.name, "testdata", "fixtures", "*.json"))
		var real []string
		for _, p := range paths {
			if filepath.Base(p) != "_frozen_pricing.json" {
				real = append(real, p)
			}
		}
		if len(real) == 0 {
			t.Logf("%-9s (khong co fixture)", v.name)
			continue
		}
		if len(real) > 12 {
			real = real[:12] // du de phu moi nhanh, khong keo dai test
		}

		pricingSource := v.pricing(t)
		checked, bad := 0, 0
		for _, fp := range real {
			raw, err := os.ReadFile(fp)
			if err != nil {
				continue
			}
			var fixture fixtureData
			if json.Unmarshal(raw, &fixture) != nil || fixture.SourcePDF == "" {
				continue
			}
			pdfPath := filepath.Join(v.name, "testdata", "realpdfs", fixture.SourcePDF)
			if _, err := os.Stat(pdfPath); err != nil {
				continue
			}

			excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
			copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)
			rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
			rows, err := rp.Process(context.Background(), pdfPath)
			if err != nil {
				continue
			}
			for _, r := range rows {
				if r.StatusKind == StatusKindFailed {
					continue
				}
				checked++
				if len(r.ExcelRows) == 0 {
					bad++
					t.Errorf("%s / %s: don %q (%s) THANH CONG nhung ExcelRows rong - push MISA se bo qua",
						v.name, fixture.SourcePDF, r.PO, r.System)
				}
				// Lop chan thu hai: co dong Excel nhung he thong chua co
				// trong bang dinh tuyen thi modal push khoa nut, khong day
				// duoc don nao ca lo.
				key := misapush.RouteKey(r.System, r.MaKhachHang, r.ShipTo)
				if br := misapush.Lookup(misapush.SeedRouting(), key); br == "" {
					noRoute[key]++
				}
			}
		}
		t.Logf("%-9s %3d don thanh cong, %d thieu ExcelRows", v.name, checked, bad)
	}

	for key, n := range noRoute {
		t.Errorf("khoa dinh tuyen %q (%d don) KHONG co trong SeedRouting - modal push se khoa nut", key, n)
	}
}
