package processing

// PriceMismatchDetail is ONE product flagged as a price mismatch —
// surfaced so the user can review it after processing and choose, per
// SKU, whether to keep the PO's own invoice price or the system's
// computed price (see excelwriter.ConfirmPrice). ExcelRow is the real
// row number in the "Don dat hang" sheet this product was written to —
// required to edit the right cell later, since nothing else in the
// returned OrderRow identifies individual product rows. Qty is this
// SKU's order quantity — the frontend needs it (alongside
// InvoicePrice/SystemPrice) to recompute the order's own DonGia total
// after the user confirms a price choice, mirroring exactly how this
// same totalValue += finalPrice * product.Qty accumulation happens on
// the Go side during the original Process() call (SystemPrice here IS
// that finalPrice, so it's already included in the OrderRow's DonGia
// exactly once — the frontend only needs to add the delta between the
// confirmed price and SystemPrice, times Qty, not recompute the whole
// total from scratch).
type PriceMismatchDetail struct {
	SKU          string  `json:"sku"`
	ProductName  string  `json:"productName"`
	InvoicePrice float64 `json:"invoicePrice"`
	SystemPrice  float64 `json:"systemPrice"`
	Qty          float64 `json:"qty"`
	ExcelRow     int     `json:"excelRow"`

	// PromoText names the promotion (already truncated via
	// truncatePromoText, same helper formatSkuLogLine's own "đã thử KM:"
	// log line uses) that was examined while computing SystemPrice, if
	// any — empty when SystemPrice is just the plain price-sheet price
	// with no promo involved. Lets the frontend explain WHY the system
	// price is what it is next to that value (see zaloMessage.ts),
	// instead of showing a bare, unexplained number that happens to
	// differ from the PO's own invoice price.
	PromoText string `json:"promoText"`

	// PromoDateRange is that same promotion's own pricing-sheet column
	// header (e.g. "04/08-09/09") — the exact value formatSkuLogLine's
	// "(áp dụng %s)" suffix already shows for a MATCHED price; surfaced
	// here too so a mismatched SKU's promo can be looked up/verified
	// against the real pricing sheet later (which column, which date
	// range), not just identified by its free-text description. Empty
	// whenever PromoText is empty (no promo was examined at all).
	PromoDateRange string `json:"promoDateRange"`
}

// PromoItemSummary is one promotional/bonus SKU's TOTAL quantity across
// the whole order, grouped by SKU (a promo can be triggered by more than
// one purchased line, e.g. the same gift-with-purchase item earned from
// two different products) — built by accumulatePromoItem as each
// buildPromoBonusRow/buildInvoiceBonusRow (or BigC's own equivalent
// inline logic) bonus row is added, keyed by that row's own SKU exactly
// as written to Excel (a promo occasionally resolves to more than one
// matched SKU joined as "sku1, sku2" — grouped as that same joined
// string, matching the granularity Excel's own SKU column already
// uses, not split further).
type PromoItemSummary struct {
	SKU         string  `json:"sku"`
	ProductName string  `json:"productName"`
	Qty         float64 `json:"qty"`
}

// OrderRow là một dòng trong bảng kết quả, ánh xạ đúng các cột của bảng
// gốc: Tên file, Trang, Hệ thống, Mã khách hàng, PO, Đơn giá, Trạng thái.
type OrderRow struct {
	FileName    string `json:"fileName"`
	SourceID    string `json:"sourceId"`
	Page        string `json:"page"`
	System      string `json:"system"`
	MaKhachHang string `json:"maKhachHang"`
	PO          string `json:"po"`
	ResultKey   string `json:"resultKey"`
	MaVanDon    string `json:"maVanDon"`
	DonGia      string `json:"donGia"`
	Status      string `json:"status"`
	StatusKind  string `json:"statusKind"`
	ExcelRows   []int  `json:"excelRows"`
	JITPeriod   string `json:"jitPeriod"`

	// DriveURL is the constructed "view" link from driveupload.Upload -
	// populated the moment a row is built (fire-and-forget: the real
	// upload may still be in progress or even fail in the background,
	// this URL is a best-effort placeholder from the start). Empty
	// string if the row's file was never uploaded (e.g. a Failed row
	// with no successfully-written Excel data to link to). Kept for the
	// Drive upload that still runs in the background — no longer
	// surfaced as its own clickable column in the frontend (the user has
	// a separate viewing mechanism for that now), only used internally.
	DriveURL string `json:"driveUrl"`

	// ShipTo/EntryDate/CancelDate/TotalWeightKg/TotalPackages mirror the
	// fields xulydonhang.py's ghi_message calls write into message.txt
	// for the eventual Zalo notification (store/entry_date/cancle_date/
	// tong_trongluong/tong_kienhang) - added so the frontend can build
	// that same message content as a preview popup without a real Zalo
	// send integration existing yet. TotalWeightKg is pre-formatted
	// (coop.FormatWeightKg: kg below 1000, tấn at/above) since that
	// formatting is already computed once per order for
	// headerDescription; TotalPackages is the running sum of each
	// product line's case count (ceil(qty/packSize), matching Python's
	// sokienhang += math.ceil(...) exactly), including promo/invoice
	// bonus rows wherever those also contribute to TotalWeightKg.
	ShipTo        string `json:"shipTo"`
	EntryDate     string `json:"entryDate"`
	CancelDate    string `json:"cancelDate"`
	TotalWeightKg string `json:"totalWeightKg"`
	TotalPackages int    `json:"totalPackages"`

	// PromoItems is every promotional/bonus SKU this order earned,
	// totaled across the whole order (see PromoItemSummary's own doc
	// comment) — surfaced for the same Zalo-notification-preview reason
	// as ShipTo/EntryDate/etc above. Always built via finalizePromoItems
	// (processor_shared.go), which uses make(...) rather than a nil
	// slice literal, so this serializes as JSON "[]" rather than "null"
	// when an order earns no bonus items — the frontend's TypeScript
	// type declares this as a plain array with no null/undefined case.
	PromoItems []PromoItemSummary `json:"promoItems"`

	// PriceMismatchCount is the same "saigia" count already embedded in
	// Status's text ("... - Có N mã sai giá") — surfaced as its own typed
	// field so the frontend can render a dedicated price-reconciliation
	// column without parsing the number back out of a display string. 0
	// for a StatusKindFailed row (pricing was never evaluated), not a
	// meaningful "all correct" signal in that case — the frontend must
	// check StatusKind first.
	PriceMismatchCount int `json:"priceMismatchCount"`

	// SkuLog holds one formatted diagnostic line per product this row's
	// underlying file/page produced (see formatSkuLogLine in
	// processor_shared.go): price-match status and any promotion
	// detected. Populated by RealProcessor's vendor-specific segment
	// handlers as they compute matched/khuyenmai for each product — not
	// a new computation, just surfacing values already derived for the
	// Excel write. Not part of the Excel output or any golden fixture;
	// app.go emits these via "process:log" before this row's own
	// "process:row" event, so they're not serialized to JSON either.
	SkuLog []string `json:"-"`

	// PriceMismatchDetails holds one entry per product flagged as a
	// price mismatch in this row's file/page — unlike SkuLog, this IS
	// serialized to JSON (sent to the frontend as part of "process:row"),
	// since the frontend needs it to let the user review/resolve each
	// mismatch after processing.
	PriceMismatchDetails []PriceMismatchDetail `json:"priceMismatchDetails"`
}

// Các giá trị Status giữ nguyên ký hiệu (emoji) của bản gốc để hiển thị
// đúng ngữ nghĩa cũ; StatusKind là discriminator kiểu (typed) dùng để
// frontend phân loại màu/icon mà không phụ thuộc vào khớp chuỗi con emoji.
const (
	StatusProcessing = "⏳ Đang xử lý"
	StatusDone       = "✅ Hoàn Thành"
	StatusWarning    = "⚠️ Hoàn Thành"
	StatusFailed     = "❌ Thất bại"

	StatusKindProcessing = "processing"
	StatusKindDone       = "done"
	StatusKindWarning    = "warning"
	StatusKindFailed     = "failed"
)
