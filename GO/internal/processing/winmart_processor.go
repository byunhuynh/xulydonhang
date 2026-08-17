package processing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"order-processor/internal/processing/coop"
	"order-processor/internal/processing/excelwriter"
	"order-processor/internal/processing/winmart"
)

// winmartRegionInfo mirrors the kho/khuvuc/mien branching inline in
// write_to_dondathang_winmart (xulydonhang.py:4226-4238) — a 3-way
// result the shared regionInfo (processor_shared.go) does NOT correctly
// cover: shared regionInfo's non-MB branch returns warehouse "LA_TP",
// but Winmart's own non-MB branch needs "LA_KHO2026" — confirmed by
// direct source comparison during planning, not the same value Coop/
// Satra use. Also has one exact-match override no other vendor has:
// customer code literally "MN_MT_WIN1326" always resolves to Đà Nẵng
// (khuvuc "MT_MN", kho "TP_DN_1", mien "DN"), checked AFTER (and
// overriding) the MB/else branch in Python — mirrored here as a switch
// with the exact-match case checked first, which is equivalent without
// needing sequential mutation: the literal "MN_MT_WIN1326" never starts
// with "MB", so the MB branch could never have produced this case's
// result in the first place, and checking the exact match first vs.
// last cannot change the outcome for any real customer code.
func winmartRegionInfo(customerCode string) (region, statCode, warehouse string) {
	switch {
	case customerCode == "MN_MT_WIN1326":
		return "MT_MN", "DN", "TP_DN_1"
	case strings.HasPrefix(customerCode, "MB"):
		return "MT_MB", "HN", "TP_HN_12"
	default:
		return "MT_MN", "LA", "LA_KHO2026"
	}
}

// winmartOrderNumber mirrors write_to_dondathang_winmart's order-number
// field (xulydonhang.py:4219,4264): f'ĐĐH{vendor}{STT_donhang_str}'
// where vendor is the uppercased literal "WINMART" and STT_donhang_str
// is f"-{po_number}" — captured from the ORIGINAL po_number, BEFORE any
// ghichu (note) text is appended to it for the L-column description
// (xulydonhang.py:4255-4256 reassigns po_number for diengiai's sake
// only, AFTER STT_donhang_str is already built) — so the order number
// itself never includes the note text, even when one is present.
func winmartOrderNumber(poNumber string) string {
	return fmt.Sprintf("ĐĐHWINMART-%s", poNumber)
}

// processWinmartSegment mirrors the Winmart branch of process_file
// (xulydonhang.py:8984-9160) plus write_to_dondathang_winmart
// (:4203-4579). Winmart is "1 page = 1 order", the same family as Coop/
// Lotte/Satra (confirmed during planning: trailing PDF pages that lack
// Winmart's identify marker are silently skipped as "Unknown" by the
// existing per-page dispatch loop, exactly as intended — no BigC-style
// whole-document state is needed). The per-item promo bonus-row block
// reuses the shared buildPromoBonusRow helper (same field mapping, same
// "AO on product row, AP on both rows" placement, same X+1
// quantity-divide logic as Coop's index==0 case) but overrides its
// no-{...}-brace fallback at the call site below — see that call site's
// own comment, and this repo's identical fix already applied to Lotte
// (lotte_processor.go), for why the shared helper's Coop-flavored
// default isn't a fit here. Winmart has no multi-CTKM-per-item loop
// (xulydonhang.py's per-item block never splits khuyenmai on "|"), so
// there is only ever one bonus attempt per item, always at index 0. The
// invoice-level ("Hóa Đơn") bonus row does NOT reuse buildInvoiceBonusRow,
// because Winmart's version writes only the FIRST matched SKU to column
// Q (xulydonhang.py:4537, kiemtra[0]), not buildInvoiceBonusRow's
// comma-joined list of every matched SKU — the same divergence BigC's
// invoice-level block had, confirmed independently for Winmart by direct
// source comparison during planning.
func (p *RealProcessor) processWinmartSegment(filePath, text, pageLabel string) (OrderRow, error) {
	poNumber, entryDate, cancelDate, note, ok := winmart.ParseOrderInfo(text)
	if !ok {
		return OrderRow{}, fmt.Errorf("không tách được số PO")
	}
	deliveryAddress, _ := winmart.ParseDeliveryAddress(text) // best-effort, matches Python's diachigiaohang staying None
	fuzzyAddress, _ := winmart.ParseFuzzyMatchAddress(text)  // best-effort, matches Python's diachi staying None

	products := winmart.ExtractProducts(text)
	if len(products) == 0 {
		return OrderRow{}, fmt.Errorf("không trích xuất được sản phẩm nào")
	}

	customerCode, found := p.Store.GetCustomerCodeByFuzzyAddress("WINMART", fuzzyAddress)
	if !found {
		customerCode = "Không xác định"
	}

	priceIndex, err := p.Pricing.FetchIndex("WINMART")
	if err != nil {
		return OrderRow{}, fmt.Errorf("không tải được giá/khuyến mãi: %w", err)
	}

	region, statCode, warehouse := winmartRegionInfo(customerCode)
	orderNum := winmartOrderNumber(poNumber)

	// diengiai (xulydonhang.py:4255-4258): built from po_number AFTER the
	// note (ghichu) is appended, if present — so the L-column description
	// includes the note, but the order number (built above, from the
	// original po_number) never does.
	descriptionPO := poNumber
	if note != "" {
		descriptionPO = fmt.Sprintf("%s - %s", poNumber, note)
	}
	description := fmt.Sprintf("WINMART PO%s", descriptionPO)
	// The header row's S column (xulydonhang.py:4275) re-splits the
	// note-appended po_number on the first "-" and keeps only what's
	// before it — for a real Winmart PO number (always plain digits,
	// confirmed during planning) this recovers the original po_number
	// EXCEPT for a trailing space left over from the " - <note>"
	// separator (e.g. "4194002858 ", not "4194002858") — Python's own
	// `.split('-')[0]` has this exact same imprecision, so it is
	// faithfully reproduced here, not "fixed" to trim it or to just
	// reuse the original po_number directly. If a future PO number ever
	// contains a literal "-" itself, this would also truncate early,
	// again matching Python's own behavior exactly (not something to
	// guard against here).
	headerProductName := fmt.Sprintf("WINMART PO%s", strings.SplitN(descriptionPO, "-", 2)[0])

	var rows []excelwriter.Row
	rows = append(rows, excelwriter.Row{
		EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
		Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
		Description: description, Warehouse: warehouse, VATPercent: 8, RegionCode: region,
		StatCode: statCode, IsNoteRow: true, ProductName: headerProductName,
	})

	saigia := 0
	totalWeight := 0.0
	totalValue := 0.0

	for _, rawProduct := range products {
		barcode := p.Store.ResolveSku(rawProduct.Barcode)
		productInfo, _ := p.Store.GetProductInfo(barcode)

		ouQty := parseNumericField(rawProduct.OUQty)
		totalPrice := parseNumericField(rawProduct.TotalPrice)

		// Zero-price "giao rời" skip (xulydonhang.py:4299-4304): Python's
		// condition is an ABSOLUTE sheet row number ("current_row - 2 >=
		// 9"), which in a real production sheet is nearly always true
		// regardless of THIS order's own row count — meaning a zero-price
		// item that's the first or second product of its own order would
		// read/overwrite a PREVIOUS order's AO/AP cells. This port
		// deliberately checks only THIS order's own accumulated rows
		// instead (see this plan's Global Constraints) — if there's no
		// prior row in `rows` to mark (fewer than 2 rows accumulated so
		// far, i.e. only the header row or nothing yet), skip the
		// zero-price item cleanly rather than reaching outside this
		// order's row set.
		if totalPrice == 0 {
			if len(rows) >= 3 { // header row + at least 2 product/bonus rows already appended
				rows[len(rows)-2].PromoNote = "KM Giao Rời - Không Che"
				rows[len(rows)-2].PromoBundleSku = ""
				rows[len(rows)-1].PromoBundleSku = ""
			}
			continue
		}

		lineWeight := productInfo.WeightKg * ouQty
		caseCount := 0
		if productInfo.PackSize > 0 {
			caseCount = int(math.Ceil(ouQty / productInfo.PackSize))
		}
		totalWeight += lineWeight

		// giahoadon (xulydonhang.py:4347-4351): WINMART divides the
		// line's total price by quantity to get a per-unit invoice
		// price — the sibling "BC MART" branch (out of scope for this
		// plan, BC Mart isn't a ported vendor) uses the total AS the
		// unit price directly instead. Only the WINMART branch is
		// ported/tested here.
		invoicePrice := totalPrice / ouQty

		realPriceStr, _ := priceIndex.FindPrice(barcode)
		realPrice := parseNumericField(realPriceStr)

		promos := priceIndex.FindPromotions(barcode, entryDate)
		khuyenmai := ""
		matched := false
		finalPrice := realPrice

		for _, promo := range promos {
			value := promo.Value
			khuyenmai = value
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
		if len(promos) == 0 && closeEnough(invoicePrice, finalPrice) {
			matched = true
		}

		productRow := excelwriter.Row{
			EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
			Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
			Description: description, SKU: barcode, Warehouse: warehouse, VATPercent: 8,
			RegionCode: region, StatCode: statCode, Qty: ouQty, UnitPrice: finalPrice,
			ProductName: productInfo.Name, CaseCount: caseCount, LineWeightKg: lineWeight, UseZFormula: true,
			PromoContent: khuyenmai,
		}
		if !matched {
			productRow.PriceMismatch = true
			productRow.InvoicePrice = invoicePrice
			saigia++
		}

		productRowIndex := len(rows)
		rows = append(rows, productRow)
		totalValue += finalPrice * ouQty

		// Per-item promo bonus row via the shared buildPromoBonusRow
		// helper (see processWinmartSegment's doc comment for the general
		// shape). write_to_dondathang_winmart's own no-brace fallback
		// (xulydonhang.py:4477-4485's "else: sheet[f'AO{current_row}'] =
		// 'KM Giao Rời - Không Che Barcode'") never writes AP at all —
		// unlike buildPromoBonusRow's Coop-flavored no-brace default
		// ("KM Bó Kèm - Che Barcode", xulydonhang.py:1198), which DOES
		// write AP on both rows and is verified, already-shipped Coop
		// behavior that must stay unchanged for Coop's own call site.
		// This is the exact same divergence already fixed for Lotte
		// (lotte_processor.go's own call site, xulydonhang.py:2204-2217)
		// — override the shared helper's result here, scoped to Winmart
		// only, rather than changing buildPromoBonusRow itself.
		bonusRow, mainRowNote, mainRowBundleSku, added := buildPromoBonusRow(p.Store, khuyenmai,
			coop.Product{Barcode: barcode, Qty: ouQty}, 0, entryDate, cancelDate, deliveryAddress,
			customerCode, description, warehouse, region, statCode, orderNum)
		if added {
			totalWeight += bonusRow.LineWeightKg

			if coop.ExtractBraceContent(khuyenmai) == "" {
				mainRowNote = "KM Giao Rời - Không Che Barcode"
				mainRowBundleSku = ""
				bonusRow.PromoBundleSku = ""
			}

			rows[productRowIndex].PromoNote = mainRowNote
			if mainRowBundleSku != "" {
				rows[productRowIndex].PromoBundleSku = mainRowBundleSku
			}
			rows = append(rows, bonusRow)
		}
	}

	// Invoice-level ("Hóa Đơn") promo bonus row (xulydonhang.py:4521-4562).
	// Does NOT reuse the shared buildInvoiceBonusRow — see this function's
	// doc comment for why (Q column gets only the first matched SKU, not
	// a joined list).
	if invoicePromo := priceIndex.FindInvoicePromotion(entryDate); invoicePromo != "" {
		invoiceSkus := p.Store.FindSkusMentioned(invoicePromo)
		if amount, ok := coop.ExtractMoneyAmount(invoicePromo); ok && amount > 0 && len(invoiceSkus) > 0 {
			invoiceSku := invoiceSkus[0] // xulydonhang.py:4537 — kiemtra[0], not a joined list
			soluongkm := math.Floor(totalValue / float64(amount))
			invoiceInfo, _ := p.Store.GetProductInfo(invoiceSku)
			invoiceWeight := invoiceInfo.WeightKg * soluongkm
			invoiceCase := 0
			if invoiceInfo.PackSize > 0 {
				invoiceCase = int(math.Ceil(soluongkm / invoiceInfo.PackSize))
			}
			totalWeight += invoiceWeight

			invoiceNote := coop.ExtractBraceContent(invoicePromo)
			if invoiceNote == "" {
				invoiceNote = "KM Bó Kèm - Che Barcode" // xulydonhang.py:4558
			}

			rows = append(rows, excelwriter.Row{
				EntryDate: entryDate, DebtDays: coopDebtDays, OrderNumber: orderNum,
				Status: "Chưa thực hiện", CancelDate: cancelDate, ShipTo: deliveryAddress, CustomerCode: customerCode,
				Description: description, SKU: invoiceSku, Warehouse: warehouse, VATPercent: 8,
				RegionCode: region, StatCode: statCode, IsPromoItem: true, Qty: soluongkm,
				ProductName: invoiceInfo.Name, CaseCount: invoiceCase, LineWeightKg: invoiceWeight, UseZFormula: false,
				PromoContent: invoicePromo, PromoNote: invoiceNote,
			})
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
		FileName: filepath.Base(filePath), Page: pageLabel, System: "Winmart", MaKhachHang: customerCode,
		PO: poNumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
	}, nil
}
