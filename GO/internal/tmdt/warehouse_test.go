package tmdt

import (
	"testing"

	"order-processor/internal/processing/warehouse"
)

// TestWarehouseOf_UsesTheConfiguredCodes covers the branch that made this
// setting necessary in the first place: on 27/08/2026 the TMĐT warehouses
// had to change (TP_HN_12 -> TP_HN_13, LA_KHOTMDT -> LA_TP) while every
// retail vendor kept its own codes, and that took a code edit.
func TestWarehouseOf_UsesTheConfiguredCodes(t *testing.T) {
	r := warehouse.NewResolver(map[string]string{
		"tmdt/HN":   "TP_HN_14",
		"tmdt/khac": "LA_KHO_MOI",
	})

	shipTo, maKho, maDonVi := warehouseOf("Kho Hà Nội", r)
	if shipTo != "HN" || maKho != "TP_HN_14" || maDonVi != "TMĐT_MB" {
		t.Errorf("warehouseOf(Kho Hà Nội) = (%q, %q, %q), want (HN, TP_HN_14, TMĐT_MB)", shipTo, maKho, maDonVi)
	}

	shipTo, maKho, maDonVi = warehouseOf("Kho Long An", r)
	if shipTo != "LA" || maKho != "LA_KHO_MOI" || maDonVi != "TMĐT_MN" {
		t.Errorf("warehouseOf(Kho Long An) = (%q, %q, %q), want (LA, LA_KHO_MOI, TMĐT_MN)", shipTo, maKho, maDonVi)
	}
}

// TestWarehouseOf_WithoutSettingsKeepsTheShippedCodes locks the codes the
// app ships with, which is what every build with nothing configured — and
// every other test in this package — must keep writing.
func TestWarehouseOf_WithoutSettingsKeepsTheShippedCodes(t *testing.T) {
	if _, maKho, _ := warehouseOf("Kho Hà Nội", nil); maKho != "TP_HN_13" {
		t.Errorf("warehouseOf(Kho Hà Nội) = %q, want the shipped %q", maKho, "TP_HN_13")
	}
	if _, maKho, _ := warehouseOf("Kho Long An", nil); maKho != "LA_TP" {
		t.Errorf("warehouseOf(Kho Long An) = %q, want the shipped %q", maKho, "LA_TP")
	}
}
