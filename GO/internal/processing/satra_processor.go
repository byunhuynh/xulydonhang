package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/satra"
)

// satraProductBlockJoinPattern reconstructs the PyMuPDF-shaped "STT
// code\nbarcode" two-line product-block start satra.ExtractProducts'
// productBlockStartPattern requires, from this repo's Go PDF library's
// actual three-separate-lines output (see normalizeSatraText's doc
// comment for why this is needed).
var satraProductBlockJoinPattern = regexp.MustCompile(`(?m)^(\d+)\n(\d+)\n(\d{13})$`)

// normalizeSatraText reconciles the confirmed differences between real
// Satra PDF text as extracted by this repo's Go PDF library
// (github.com/ledongthuc/pdf) and the PyMuPDF-shaped text every function
// in the satra package (Tasks 3-4) was built and validated against.
// These were discovered only once a real sample file was run through the
// actual extraction pipeline end-to-end (extractPageTexts), not the
// synthetic/PyMuPDF-harness text Tasks 3-4's own validation used — the
// same category of gap processLotteSegment's stripBlankLines documents
// for Lotte, confirmed here to also apply to Satra's own PDF template,
// plus two further Satra-specific differences:
//
//  1. Blank lines: reuses the same stripBlankLines already shared with
//     Lotte (see lotte_processor.go) — this library's line-break
//     detection inserts blank lines PyMuPDF's does not.
//
//  2. Stray replacement characters (U+FFFD): confirmed on multiple real
//     Satra PDFs (P-005508192.pdf, P-000022974.pdf) that this library's
//     GetPlainText emits a literal U+FFFD after many label/value lines
//     (e.g. "178 500,00�") — almost certainly a glyph in this
//     vendor's PDF template with no ToUnicode mapping this library can
//     resolve, which PyMuPDF's extraction does not reproduce. Left in
//     place, the trailing byte makes strconv.ParseFloat fail on every
//     price line inside satra.ExtractProducts, silently dropping every
//     product. It never carries real data, so it is stripped outright.
//
//  3. Product-block line grouping: satra.ExtractProducts' regex expects
//     a line pairing "STT code" (e.g. "1 300867") together on ONE line,
//     immediately followed by the 13-digit barcode on the next —
//     matching PyMuPDF's row-grouping for this table. This library's
//     GetPlainText instead puts STT, product code, and barcode each on
//     their OWN line (confirmed identically on both real sample files
//     above, once blank lines are stripped: every product block starts
//     with exactly three consecutive single-token lines). Re-merging
//     the first two of those three lines reconstructs the exact shape
//     satra.ExtractProducts already expects, without needing to touch
//     that package's already-shipped, Task-4-reviewed regex itself.
func normalizeSatraText(text string) string {
	text = strings.ReplaceAll(text, "�", "")
	text = stripBlankLines(text)
	return satraProductBlockJoinPattern.ReplaceAllString(text, "$1 $2\n$3")
}

// satraOrderNumber mirrors write_to_dondathang_satra's order-number
// field (xulydonhang.py:2379): f'ĐĐH{vendor}{STT_donhang_str}' where
// vendor is the uppercased literal "SATRA" and STT_donhang_str is
// f"-{po_number}".
func satraOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHSATRA-%s", poNumber)
}

// noSaturdayDeliveryCustomerCode mirrors the one hardcoded special case
// in write_to_dondathang_satra (xulydonhang.py:2371-2372): this specific
// customer code's order description gets a "- Không giao thứ 7" suffix.
const noSaturdayDeliveryCustomerCode = "MN_MT_stph"

// processSatraSegment mirrors the Satra branch of process_file
// (xulydonhang.py:9303-9394) plus write_to_dondathang_satra
// (:2330-2692). Unlike processLotteSegment, this needs NO overrides on
// buildPromoBonusRow/buildInvoiceBonusRow — Satra's promo-matching and
// bonus-row logic (xulydonhang.py:2555-2672) is structurally identical
// to Coop's write_to_dondathang (same khuyenmai.split('|') + enumerate
// loop, same "KM Bó Kèm - Che Barcode" no-brace default), confirmed by
// direct source comparison during planning — so this mirrors
// processSegment's promo loop shape, not processLotteSegment's
// single-call shape.
func (p *RealProcessor) processSatraSegment(filePath, text, pageLabel string) (OrderRow, error) {
	text = normalizeSatraText(text)

	poNumber, ok := satra.ParsePONumber(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO")
	}
	entryDate, ok := satra.ParseEntryDate(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không đọc được ngày đặt hàng")
	}
	cancelDate, _ := satra.ParseCancelDate(text) // best-effort, matches Python's silent-if-missing behavior
	shipTo, _ := satra.ParseShipToAddress(text)  // best-effort

	products := satra.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	customerCode, found := p.Store.GetCustomerCodeByFuzzyAddress("SATRA", shipTo)
	if !found {
		customerCode = "Không xác định"
	}

	priceIndex, err := p.Pricing.FetchIndex("SATRA")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := regionInfo(customerCode)
	description := fmt.Sprintf("SATRA %s", poNumber)
	if customerCode == noSaturdayDeliveryCustomerCode {
		description += " - Không giao thứ 7"
	}

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: satraOrderNumber(poNumber),
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: description,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)
		qty := rawProduct.Qty
		lineWeight := productInfo.WeightKg * qty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(qty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		invoicePrice := rawProduct.TotalPrice
		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice, _ := strconv.ParseFloat(strings.ReplaceAll(realPriceStr, ",", ""), 64)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		lastExaminedPromo := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := promo.Value
			lastExaminedPromo = value
			if value == "" {
				continue
			}
			candidatePrice := realPrice
			if discount := coop.ExtractDiscount(value); discount != 0 {
				candidatePrice = realPrice - (realPrice * discount / 100)
			}
			finalPrice = candidatePrice
			if closeEnough(invoicePrice, candidatePrice) {
				matched = true
				break
			}
		}
		if len(promos) == 0 && closeEnough(invoicePrice, realPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: satraOrderNumber(poNumber),
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: shipTo, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: qty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: lastExaminedPromo,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * qty

		// Satra's promo/bonus-row loop DOES split on "|" and DOES use
		// buildPromoBonusRow's own default — no override needed, unlike
		// Lotte (xulydonhang.py:2555 confirms the same khuyenmai.split('|')
		// Coop uses; the AQ-write-every-iteration / i==0-vs-i>0 off-by-one
		// this mirrors is the same quirk documented in detail on Coop's
		// equivalent loop in coop_processor.go).
		currentRowIndex := productRowIndex
		for i, promoPart := range strings.Split(lastExaminedPromo, "|") {
			rows[currentRowIndex].PromoContent = lastExaminedPromo

			bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, promoPart,
				coop.Product{Barcode: barcode, Qty: qty}, i, entryDate, cancelDate, shipTo,
				customerCode, description, warehouse, region, statCode, poNumber)
			if !added {
				continue
			}
			bonusRow.OrderNumber = satraOrderNumber(poNumber) // buildPromoBonusRow hardcodes Coop's order number
			totalWeight += bonusRow.LineWeightKg
			if i == 0 {
				rows[productRowIndex].PromoNote = mainRowNote
				if mainRowBundleSku != "" {
					rows[productRowIndex].PromoBundleSku = mainRowBundleSku
				}
			}
			rows = append(rows, bonusRow)
			currentRowIndex = len(rows) - 1
		}
	}

	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		if bonusRow, added := buildInvoiceBonusRow(p.Store, invoicePromo, totalValue, entryDate, cancelDate,
			shipTo, customerCode, description, warehouse, region, statCode, poNumber); added {
			bonusRow.OrderNumber = satraOrderNumber(poNumber)
			totalWeight += bonusRow.LineWeightKg
			rows = append(rows, bonusRow)
		}
	}

	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	if err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription); err != nil {
		return OrderRow{}, err
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Satra", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
