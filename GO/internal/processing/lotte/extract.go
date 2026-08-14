package lotte

import (
	"fmt"
	"strings"
	"time"
)

// OrderInfo holds the PO number, entry date, and store code derived from
// a Lotte page's first two lines.
type OrderInfo struct {
	PONumber  string // formatted "yyMMdd-STORECODE-ORDER", e.g. "260727-01013-00057"
	EntryDate string // "dd/MM/yyyy"
	StoreCode string // the 5-digit middle segment of PONumber, e.g. "01013"
}

// ParseOrderInfo mirrors the PO-number/entry-date derivation at the top
// of process_file's Lotte branch (xulydonhang.py:9081-9092): the raw PO
// number is the page's SECOND line (index 1) — a 16-digit string with no
// separators (6-digit date + 5-digit store code + 5-digit order number).
// Hyphens are inserted at fixed byte offsets 6 and 12 to produce the
// formatted PO number, which is then split on "-" to recover each part.
//
// Deliberately more defensive than the Python original here: Python's
// po_number.split("-") is unpacked directly into 3 variables and raises
// an unhandled ValueError if the line doesn't produce exactly 3 hyphen-
// separated parts (e.g. too short to reach either insertion point) —
// this returns an error instead, per this plan's "correct main flow,
// don't need bug-for-bug parity" policy. Every real sample so far
// produces exactly 3 parts.
func ParseOrderInfo(text string) (OrderInfo, error) {
	lines := strings.Split(text, "\n")
	raw := ""
	if len(lines) > 1 {
		raw = lines[1]
	}

	poNumber := raw
	if len(poNumber) >= 7 {
		poNumber = poNumber[:6] + "-" + poNumber[6:]
	}
	if len(poNumber) >= 12 {
		poNumber = poNumber[:12] + "-" + poNumber[12:]
	}

	parts := strings.Split(poNumber, "-")
	if len(parts) != 3 {
		return OrderInfo{}, fmt.Errorf("không tách được số PO từ dòng thứ 2 của trang: %q", raw)
	}
	timePart, storeCode := parts[0], parts[1]

	entryDate, err := time.Parse("060102", timePart)
	if err != nil {
		return OrderInfo{}, fmt.Errorf("không đọc được ngày đặt hàng từ %q: %w", timePart, err)
	}

	return OrderInfo{
		PONumber:  poNumber,
		EntryDate: entryDate.Format("02/01/2006"),
		StoreCode: storeCode,
	}, nil
}
