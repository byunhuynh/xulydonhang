// Builds the Zalo order-notification message shown to the recipient
// (store/vendor contact). Content uses the markup syntax
// GO/internal/zalosend/richtext understands (RICH_TEXT_SYNTAX.md -
// **bold**/*italic*/__underline__/~~strike~~/{color:text}/list) - the
// real send (ChromedpSender.SendMessage) always pastes through that
// engine regardless of whether markup is present, so using it costs
// nothing and makes the notification much easier for a recipient to
// scan (bold PO/total, colored price-mismatch warning, a real bullet
// list for promo items) than a wall of plain emoji-prefixed lines.
// The preview modal renders this SAME markup through
// lib/richtext.ts's smaller TS port, so what's previewed always
// matches what actually gets sent. Grouped into short sections (blank
// line between each) rather than one dense block, and keeps the
// price-mismatch detail the recipient actually needs: not a raw
// invoice-vs-system dump, but which price basis is CURRENTLY being used
// for each mismatched SKU (and for the order's own total) — since that
// choice is exactly what changes what they'll actually be charged.
import type { OrderRow, PriceMismatchDetail } from "../types";

// Tiền tố SourceID của dòng tóm tắt TMĐT (backend đặt "tmdt|{shop}" —
// xem summaryTMDTRows trong GO/app_tmdt.go); phần sau tiền tố là tên shop.
//
// Khai ở ĐÂY chứ không ở zaloGrouping.ts (nơi hợp lý hơn về mặt ý nghĩa)
// vì file này phải nạp được bằng `node --test`: tsconfig loại test khỏi
// tsc nên test import kèm đuôi ".ts", còn mã nguồn thì không được — nghĩa
// là zaloMessage.ts không thể import runtime từ file khác trong lib mà
// vẫn chạy được dưới node. Chiều ngược lại (zaloGrouping import từ đây)
// thì an toàn: không test nào nạp zaloGrouping trực tiếp.
export const TMDT_SOURCE_PREFIX = "tmdt|";

// MANUAL_ENTRY_PAGE_MARKER khớp Page: "nhập tay" mà
// manualentry_processor.go gắn cho MỌI đơn xử lý qua "đơn hàng tay.xlsx"
// (chỉ hiện trên UI, không ghi vào Excel - xem comment ở đó). Đơn nhập
// tay không có PDF nguồn nào được tải lên Drive, nên link tra cứu
// "bluedonhang.pages.dev/?po=..." (trỏ tới trang tra soát dựng từ PDF
// gốc) không có ý nghĩa gì cho loại đơn này - dùng marker này để ẩn hẳn
// dòng link khỏi tin Zalo thay vì hiện 1 link chết.
const MANUAL_ENTRY_PAGE_MARKER = "nhập tay";

function isManualEntryRow(row: OrderRow): boolean {
  return row.page === MANUAL_ENTRY_PAGE_MARKER;
}

/**
 * tmdtShopFromGroupKey trả tên shop của một nhóm TMĐT, hoặc chuỗi rỗng khi
 * key không phải nhóm TMĐT — dùng chung ở mọi nơi cần hỏi "nhóm này có
 * phải TMĐT không" (ResultTable dựng nhãn, ControlPanel dựng job gửi,
 * OrderContentModal dựng bản xem trước) để ba nơi đó không tự cắt chuỗi
 * mỗi nơi một kiểu.
 */
export function tmdtShopFromGroupKey(key: string): string {
  return key.startsWith(TMDT_SOURCE_PREFIX)
    ? key.slice(TMDT_SOURCE_PREFIX.length)
    : "";
}

function formatMoney(n: number): string {
  return n.toLocaleString("vi-VN");
}

// bold wraps a value in **markup** only when non-empty - a truthy check
// is enough since callers only ever pass strings here; wrapping an empty
// string would otherwise produce a bare, meaningless "****".
function bold(
  value: string | number | null | undefined,
): string | number | null | undefined {
  if (value === "" || value === null || value === undefined) return value;
  return `**${value}**`;
}

export type PriceBasis = "po" | "system";

// DETAIL_INDENT thụt lề các dòng chi tiết (giá PO/hệ thống dưới 1 SKU
// sai giá) bằng KÝ TỰ KHOẢNG TRẮNG KHÔNG NGẮT (U+00A0) lặp lại, KHÔNG
// dùng cú pháp "2 khoảng trắng + list lồng nhau" của RICH_TEXT_SYNTAX.md
// mục 3 nữa. Lý do đổi: cách cũ dựa vào ChromedpSender bấm nút thật
// "Lùi đầu dòng" N lần SAU khi dán (applyIndents, cơ chế hybrid 2 bước)
// — xác nhận qua gửi thật là KHÔNG ổn định cho danh sách nhiều mã sai
// giá liên tiếp (thụt lề bị làm phẳng). Non-breaking space thì ngược
// lại: là TEXT THẬT nằm sẵn trong nội dung dán, sống sót qua HTML y hệt
// nhau ở cả bản xem trước (richtext.ts) lẫn bản dán thật (không giống
// khoảng trắng thường U+0020, trình duyệt/HTML KHÔNG gộp/rút gọn nbsp
// liên tiếp) — không còn phụ thuộc bước bấm nút nào cả, đơn giản hơn và
// đáng tin cậy hơn hẳn.
const DETAIL_INDENT = "    ";

// DIVIDER phân cách các phần của tin (tiêu đề/tổng tiền/cảnh báo giá/
// khuyến mãi/link) — 12 ký tự "━", ĐÃ gửi thử thật lên điện thoại để chọn
// độ dài: 20 ký tự bị tràn 1-2 ký tự cuối xuống dòng riêng trên màn hình
// hẹp, 12 ký tự thì không.
const DIVIDER = "━━━━━━━━━━━━";

// code tô màu cam cho mã hàng (SKU) — tách biệt rõ với tên sản phẩm
// (giữ đậm, không màu) khi cùng xuất hiện trên 1 dòng: không thể vừa
// đậm vừa màu trên CÙNG 1 đoạn (RICH_TEXT_SYNTAX.md mục 1, không lồng
// định dạng), nên SKU dùng {orange:...} còn tên sản phẩm dùng **...**
// — 2 kiểu định dạng khác nhau vẫn tạo được 2 điểm nhấn thị giác riêng
// biệt trên cùng 1 dòng. Không dùng đỏ/xanh lá ở đây vì 2 màu đó đã
// mang nghĩa cố định (cảnh báo sai giá / giá đang áp dụng) trong
// formatMismatchLine — dùng lại cho SKU sẽ gây hiểu nhầm.
function code(sku: string): string {
  return `{orange:${sku}}`;
}

// formatMismatchLine dựng khối cho 1 SKU sai giá: dòng cha (mã hàng tô
// cam + tên đậm, không gạch đầu dòng "- " vì bản thân cả khối mismatch
// không phải 1 <ul>, khác promo bên dưới) và 1 dòng con thụt lề bằng
// DETAIL_INDENT gộp CẢ 2 giá nối bằng "→" (thay vì 2 dòng riêng như bản
// cũ) — giá KHÔNG được áp dụng gạch ngang, giá ĐANG được áp dụng tô xanh
// lá kèm ✅, mũi tên tự nói lên "giá này đổi thành giá kia". Bố cục đã
// gửi thử thật (Zalo di động) và xác nhận đọc rõ, không tràn dòng. KHÔNG
// lồng **đậm** vào bên trong {màu:...} ở đây - cú pháp không hỗ trợ lồng
// 2 định dạng cùng lúc trên 1 đoạn (RICH_TEXT_SYNTAX.md mục 1); code()
// và **productName** vì vậy tách rời trên cùng dòng thay vì gộp chung 1
// span. Dùng chung cho cả buildZaloMessage lẫn buildZaloMessageForPO
// (logic giống hệt nhau, chỉ khác nguồn dữ liệu gọi vào).
function formatMismatchLine(d: PriceMismatchDetail, basis: PriceBasis): string {
  // Names the promo behind "Hệ thống" whenever one was actually examined
  // for this SKU - a bare system price that happens to differ from the
  // PO's own invoice price reads as unexplained otherwise; "(KM: ...)" is
  // the same promo text already shown for a MATCHED price in the system
  // log (formatSkuLogLine's own "KM:" suffix), just surfaced here too
  // since a mismatch is exactly the case where the recipient most needs
  // to know a promo was involved. The trailing "(áp dụng <date range>)"
  // is that same promo's own pricing-sheet column header - lets whoever
  // reviews this later look the exact promo up on the real sheet instead
  // of hunting by free-text description alone.
  const dateSuffix =
    d.promoText && d.promoDateRange ? ` (áp dụng ${d.promoDateRange})` : "";
  const promoNote = d.promoText ? ` (KM: ${d.promoText}${dateSuffix})` : "";

  const poText = `PO ${formatMoney(d.invoicePrice)}đ`;
  const systemText = `Hệ thống ${formatMoney(d.systemPrice)}đ`;
  const poSide = basis === "po" ? `{green:✅ ${poText}}` : `~~${poText}~~`;
  // promoNote nằm NGOÀI dấu định dạng, cả nhánh gạch ngang lẫn nhánh tô
  // xanh. Dấu định dạng ở đây nói về CÁI GIÁ - gạch ngang = "giá này bị
  // bỏ", tô xanh = "giá này được áp dụng" - còn "(KM: ...)" chỉ giải
  // thích con số đó ở đâu ra. Gộp nó vào trong dấu gạch khiến cả chương
  // trình khuyến mãi trông như cũng bị bỏ theo, đúng thứ người dùng báo
  // lỗi. Để ngoài cả hai nhánh thì vị trí ghi chú không nhảy chỗ tuỳ
  // theo người dùng chọn giá nào - đọc hai đơn cạnh nhau không bị lệch.
  const systemSide =
    (basis === "system" ? `{green:✅ ${systemText}}` : `~~${systemText}~~`) +
    promoNote;

  return (
    `${code(d.sku)} — **${d.productName}**\n` +
    `${DETAIL_INDENT}${poSide} → ${systemSide}`
  );
}

// formatPromoLine dùng cú pháp gạch đầu dòng thật ("- ") thay vì ký tự
// "•" cứng như trước - GO/internal/zalosend/richtext nhận diện đúng cú
// pháp này và gộp các dòng liên tiếp thành 1 <ul> thật khi dán vào Zalo
// (xem RICH_TEXT_SYNTAX.md mục 2), thay vì chỉ là các dòng text rời rạc
// nhìn giống danh sách. Mã hàng tô cam như formatMismatchLine, NHƯNG tên
// sản phẩm KHÔNG đậm ở đây (khác mismatch) - cố ý: khuyến mãi chỉ mang
// tính thông tin, không cần hành động gì, để plain giúp phân biệt ngay
// bằng mắt với các dòng mismatch (in đậm = cần chú ý/xác nhận).
function formatPromoLine(p: {
  sku: string;
  productName: string;
  qty: number;
}): string {
  return `- ${code(p.sku)} ${p.productName}: ${p.qty}`;
}

// resolveEffectivePrice là whichever giá đang tính vào DonGia của dòng
// này cho SKU này: giá PO nếu người dùng đã xác nhận chọn, ngược lại
// giá hệ thống (mặc định của DonGia — xem PriceMismatchDetail's doc).
// resolvedChoice key là `${rowIndex}-${excelRow}` (khớp cách
// ResultTable.tsx đã dùng, giữ nguyên qua lần refactor này).
export function resolveEffectivePrice(
  rowIndex: number,
  detail: PriceMismatchDetail,
  resolvedChoice: Record<string, PriceBasis>,
): number {
  const choice = resolvedChoice[`${rowIndex}-${detail.excelRow}`];
  return choice === "po" ? detail.invoicePrice : detail.systemPrice;
}

// buildPriceBasisForRow rút gọn resolvedChoice (key theo rowIndex, có
// thể lặp excelRow giữa các dòng khác nhau) xuống 1 map theo excelRow
// riêng của 1 dòng — đúng dạng buildZaloMessage cần.
export function buildPriceBasisForRow(
  rowIndex: number,
  row: OrderRow,
  resolvedChoice: Record<string, PriceBasis>,
): Record<number, PriceBasis> {
  const result: Record<number, PriceBasis> = {};
  for (const d of row.priceMismatchDetails ?? []) {
    result[d.excelRow] =
      resolvedChoice[`${rowIndex}-${d.excelRow}`] ?? "system";
  }
  return result;
}

// buildPriceBasisForPO gộp buildPriceBasisForRow của mọi dòng thuộc 1 PO
// (BigC có thể có nhiều dòng/PO) thành 1 map duy nhất cho
// buildZaloMessageForPO — an toàn gộp vì excelRow là số dòng Excel thật,
// không bao giờ trùng giữa 2 OrderRow khác nhau.
export function buildPriceBasisForPO(
  rows: OrderRow[],
  rowIndices: number[],
  resolvedChoice: Record<string, PriceBasis>,
): Record<number, PriceBasis> {
  const result: Record<number, PriceBasis> = {};
  for (const idx of rowIndices) {
    Object.assign(
      result,
      buildPriceBasisForRow(idx, rows[idx], resolvedChoice),
    );
  }
  return result;
}

// assembleOrderMessage dựng nội dung tin theo đúng bố cục ĐÃ GỬI THỬ THẬT
// lên Zalo di động và xác nhận đọc rõ (tiêu đề gộp tên hệ thống, PO+cửa
// hàng chung 1 dòng, 2 mốc ngày chung 1 dòng, DIVIDER phân cách các phần)
// - dùng chung cho cả buildZaloMessage lẫn buildZaloMessageForPO, chỉ
// khác nguồn dữ liệu đầu vào đã được 2 hàm đó gom sẵn.
//
// Cách nối các khối: header (tiêu đề+PO/cửa hàng+ngày) luôn là 1 đoạn
// riêng. Từ khối tổng tiền trở đi, mọi khối HIỆN CÓ (tổng tiền, cảnh báo
// sai giá, khuyến mãi, link) được nối liền bằng DIVIDER (không xuống
// dòng trống) — RIÊNG bên trong khối cảnh báo sai giá, dòng cảnh báo và
// từng mã sai giá vẫn cách nhau 1 dòng trống như bản cũ (dễ phân biệt
// từng mã). Khối nào không có dữ liệu (vd không có khuyến mãi) thì tự
// biến mất khỏi chuỗi nối, không để lại DIVIDER mồ côi.
function assembleOrderMessage(fields: {
  po: string;
  system: string;
  shipTo: string;
  entryDate: string;
  cancelDate: string;
  totalMoney: number;
  totalPackages: number | string | undefined | null;
  totalWeightKg: string | undefined | null;
  mismatches: PriceMismatchDetail[];
  priceBasisBySku: Record<number, PriceBasis>;
  promoItems: { sku: string; productName: string; qty: number }[];
  orderUrl: string;
  processedAt: string;
}): string {
  const hasMismatch = fields.mismatches.length > 0;
  const hasPromo = fields.promoItems.length > 0;

  const titleSystem = fields.system ? ` ${fields.system.toUpperCase()}` : "";
  const headerLines = [`**🔔 ĐƠN HÀNG${titleSystem}**`, DIVIDER];
  const identityParts = [
    fields.po && `🎫 ${bold(fields.po)}`,
    fields.shipTo && `🏪 ${fields.shipTo}`,
  ].filter(Boolean);
  if (identityParts.length > 0) headerLines.push(identityParts.join(" · "));
  const dateParts = [
    fields.entryDate && `Đặt ${fields.entryDate}`,
    fields.cancelDate && `Hạn ${fields.cancelDate}`,
  ].filter(Boolean);
  if (dateParts.length > 0) headerLines.push(`🗓️ ${dateParts.join(" → ")}`);
  const headerBlock = headerLines.join("\n");

  const totalsParts = [
    `💰 **${formatMoney(fields.totalMoney)}đ**${hasMismatch ? " (theo giá dưới)" : ""}`,
  ];
  if (fields.totalPackages) totalsParts.push(`📦 ${fields.totalPackages} kiện`);
  if (fields.totalWeightKg) totalsParts.push(`⚖️ ${fields.totalWeightKg}`);
  const totalsBlock = totalsParts.join(" · ");

  let mismatchBlock = "";
  if (hasMismatch) {
    const items = fields.mismatches.map((d) =>
      formatMismatchLine(d, fields.priceBasisBySku[d.excelRow] ?? "system"),
    );
    mismatchBlock = [
      `{red:⚠️ Có ${fields.mismatches.length} mã chờ xác nhận giá}`,
      ...items,
    ].join("\n\n");
  }

  const promoBlock = hasPromo
    ? `**🎁 Khuyến mãi**\n${fields.promoItems.map(formatPromoLine).join("\n")}`
    : "";

  const linkBlock = fields.orderUrl ? `🔗 ${fields.orderUrl}` : "";

  const chained = [totalsBlock, mismatchBlock, promoBlock, linkBlock]
    .filter((b) => b !== "")
    .join(`\n${DIVIDER}\n`);

  const paragraphs = [headerBlock, chained].filter((p) => p !== "");
  if (fields.processedAt) paragraphs.push(`⏱️ Xử lý lúc ${fields.processedAt}`);

  return paragraphs.join("\n\n");
}

// buildZaloMessage's processedAt is the frontend's own "row just
// arrived" timestamp (stamped into appStore.receivedAt when the row first
// showed up in the table) - a fair, honest stand-in for Python's real
// server-side start_time, which the Go pipeline has no equivalent moment
// to record from. Both the preview modal and the real send read that one
// stamped value, so the message the customer receives carries exactly the
// timestamp the user reviewed.
//
// priceBasisBySku tells, for each mismatched SKU (keyed by its own
// excelRow, matching ResultTable's own resolvedChoice key), which price
// the user has currently chosen — 'system' is the correct default for
// any SKU the user hasn't touched yet, matching exactly what the
// order's own DonGia total is already computed with by default (see
// PriceMismatchDetail's own doc comment in types.go).
export function buildZaloMessage(
  row: OrderRow,
  processedAt: string,
  priceBasisBySku: Record<number, PriceBasis>,
): string {
  return assembleOrderMessage({
    po: row.po,
    system: row.system,
    shipTo: row.shipTo,
    entryDate: row.entryDate,
    cancelDate: row.cancelDate,
    totalMoney: Number(row.donGia) || 0,
    totalPackages: row.totalPackages,
    totalWeightKg: row.totalWeightKg,
    mismatches: row.priceMismatchDetails ?? [],
    priceBasisBySku,
    promoItems: row.promoItems ?? [],
    orderUrl:
      row.po && !isManualEntryRow(row)
        ? `https://bluedonhang.pages.dev/?po=${row.po}`
        : "",
    processedAt,
  });
}

// parseWeightKg/formatWeightKg mirror the Go side's coop.FormatWeightKg
// exactly (kg below 1000, tấn at/above, always one decimal place) - the
// only way to correctly SUM several rows' already-formatted weight
// strings back into one combined total without redoing the underlying
// kg math on the Go side.
function parseWeightKg(formatted: string): number {
  const n = parseFloat(formatted);
  if (Number.isNaN(n)) return 0;
  return formatted.trim().endsWith("tấn") ? n * 1000 : n;
}

function formatWeightKg(kg: number): string {
  if (kg >= 1000)
    return `${(kg / 1000).toFixed(2).replace(/0$/, "").replace(/\.$/, ".0")} tấn`;
  return `${kg.toFixed(2).replace(/0$/, "").replace(/\.$/, ".0")} kg`;
}

// buildZaloMessageForPO mirrors the real xulydonhang.py mechanism
// confirmed for BigC (xulydonhang.py:9508-9616 + write_to_dondathang_bigc
// :4925-4964): a single PO can produce several OrderRows in this port
// (one per BigC store page), but the real app writes exactly ONE
// message.txt block per PO number - opened once, added to by every
// store, closed once with a PO-wide aggregate total. Every OTHER vendor
// already has exactly one OrderRow per PO, so passing a 1-row array here
// degrades to the same output buildZaloMessage would produce alone.
//
// Price-mismatch and promo lines are merged BY SKU across every row in
// the group rather than repeated once per store (confirmed as the
// intended business rule, not a Python-parity requirement): a price
// mismatch is a property of the shared price/CTKM sheet, not of any one
// store, so the same SKU mismatched in two stores is one line with the
// quantities added together, not two identical-looking lines.
export function buildZaloMessageForPO(
  rows: OrderRow[],
  processedAt: string,
  priceBasisBySku: Record<number, PriceBasis>,
): string {
  if (rows.length === 0) return "";
  const first = rows[0];
  const orderUrl =
    first.po && !isManualEntryRow(first)
      ? `https://bluedonhang.pages.dev/?po=${first.po}`
      : "";

  const totalDonGia = rows.reduce((sum, r) => sum + (Number(r.donGia) || 0), 0);
  const totalPackages = rows.reduce(
    (sum, r) => sum + (r.totalPackages || 0),
    0,
  );
  const totalWeightKg = formatWeightKg(
    rows.reduce((sum, r) => sum + parseWeightKg(r.totalWeightKg || "0 kg"), 0),
  );

  const mismatchBySku = new Map<string, PriceMismatchDetail>();
  for (const r of rows) {
    for (const d of r.priceMismatchDetails ?? []) {
      const existing = mismatchBySku.get(d.sku);
      if (existing) existing.qty += d.qty;
      else mismatchBySku.set(d.sku, { ...d });
    }
  }
  const mismatches = [...mismatchBySku.values()];

  const promoBySku = new Map<
    string,
    { sku: string; productName: string; qty: number }
  >();
  for (const r of rows) {
    for (const p of r.promoItems ?? []) {
      const existing = promoBySku.get(p.sku);
      if (existing) existing.qty += p.qty;
      else promoBySku.set(p.sku, { ...p });
    }
  }
  const promoItems = [...promoBySku.values()];

  return assembleOrderMessage({
    po: first.po,
    system: first.system,
    shipTo: first.shipTo,
    entryDate: first.entryDate,
    cancelDate: first.cancelDate,
    totalMoney: totalDonGia,
    totalPackages,
    totalWeightKg,
    mismatches,
    priceBasisBySku,
    promoItems,
    orderUrl,
    processedAt,
  });
}

// buildZaloMessageForJITFile gộp CẢ FILE PDF JIT thành 1 tin (giống
// buildZaloMessageForPO gộp CẢ PO của BigC thành 1 tin) - nhưng KHÔNG
// dùng chung assembleOrderMessage/buildZaloMessageForPO được: 1 file JIT
// chứa NHIỀU trang, MỖI trang 1 PO khác nhau (không như BigC, các dòng
// chia sẻ CHUNG 1 po) - assembleOrderMessage chỉ hiện được đúng 1 po đại
// diện (first.po), gọi thẳng với rows JIT sẽ ÂM THẦM làm mất mọi PO còn
// lại khỏi tin nhắn. JIT cũng không có sai giá/khuyến mãi (Go processor
// không set priceMismatchDetails/promoItems cho JIT) nên không cần phần
// đó - liệt kê từng PO riêng cũng không hợp lý khi 1 file có thể có rất
// nhiều PO, nên chỉ gộp lại thành SỐ LƯỢNG (đơn/mã sản phẩm) + tổng
// tiền/cân nặng.
//
// KHÔNG hiện "số kiện" (totalPackages) - hàng JIT-CHOICE đóng thương mại
// điện tử, mỗi đơn đóng gói RIÊNG một kiện, không gộp chung nhiều đơn/
// kiện như quy tắc đóng kiện các hệ thống khác (info.PackSize) vẫn giả
// định - số kiện tính theo quy tắc đó nên KHÔNG đúng cho JIT, hiện ra chỉ
// gây hiểu nhầm.
//
// period là buổi giao (sáng/chiều) THEO GIÁ TRỊ NGƯỜI DÙNG ĐANG CHỌN -
// caller phải tự lấy jitPeriodState.periodBySource[sourceId] trước (rơi
// về row.jitPeriod nếu người dùng chưa đổi gì), KHÔNG đọc row.jitPeriod
// trực tiếp ở đây vì giá trị đó chỉ phản ánh buổi lúc xử lý PDF, có thể
// đã lỗi thời nếu người dùng vừa đổi qua ô chọn buổi trên bảng.
export function buildZaloMessageForJITFile(
  rows: OrderRow[],
  period: string,
  processedAt: string,
): string {
  if (rows.length === 0) return '';
  const first = rows[0];

  const totalDonGia = rows.reduce((sum, r) => sum + (Number(r.donGia) || 0), 0);
  const totalWeightKg = formatWeightKg(
    rows.reduce((sum, r) => sum + parseWeightKg(r.totalWeightKg || '0 kg'), 0),
  );
  // 2 con số KHÁC NHAU, dễ nhầm nên tách rõ: số MÃ HÀNG khác nhau (loại
  // trùng theo ĐÚNG mã SKU thật qua Set, KHÔNG dùng excelRows - mỗi lần
  // ghi luôn ra 1 số dòng Excel mới nên cộng dồn qua nhiều PO chỉ ra
  // tổng số DÒNG sản phẩm, không phải số SKU khác nhau - xác nhận sai
  // qua thực tế: 170 PO gần như 1 SKU/đơn từng báo nhầm "173 mã hàng")
  // KHÁC với tổng SỐ LƯỢNG sản phẩm (cộng dồn Qty qua totalQty - vd 10
  // mã có thể lên tới 15 sản phẩm nếu vài mã có Qty > 1).
  const skuSet = new Set<string>();
  // `?? []`: Go trả null cho slice chưa gán (đã kiểm: `"skus":null`),
  // đúng lý do mọi chỗ khác trong file này cũng viết `?? []`.
  for (const r of rows) for (const sku of r.skus ?? []) skuSet.add(sku);
  const totalQty = rows.reduce((sum, r) => sum + (r.totalQty || 0), 0);

  const header = `**🔔 ĐƠN HÀNG JIT-CHOICE**\n${DIVIDER}`;
  const identityLine = `🏪 {orange:${first.shipTo}} · 🗓️ ${first.entryDate} (**${period}**)`;
  const countsLine = `🎫 Tổng số đơn: **${rows.length} PO** · 🏷️ Tổng số mã hàng: **${skuSet.size} mã**`;
  const totalsLine = `💰 **${formatMoney(totalDonGia)}đ** · 📦 ${totalQty} sản phẩm · ⚖️ ${totalWeightKg}`;

  const paragraphs = [[header, identityLine].join('\n'), [countsLine, totalsLine].join('\n')];
  if (processedAt) paragraphs.push(`⏱️ Xử lý lúc ${processedAt}`);

  return paragraphs.join('\n\n');
}

// formatTMDTDateSpan gộp danh sách ngày "dd/mm/yyyy" thành một nhãn: một
// ngày thì in nguyên ngày, nhiều ngày thì in khoảng đầu → cuối. Bỏ năm ở
// đầu khoảng khi hai đầu cùng năm — dòng tiêu đề đã đủ dài rồi.
function formatTMDTDateSpan(dates: string[]): string {
  const seen = [...new Set(dates.filter((d) => d.trim() !== ""))];
  if (seen.length === 0) return "";
  // "23/08/2026" -> "20260823": so được bằng phép so chuỗi, không cần Date
  // (và không dính bẫy múi giờ như new Date("23/08/2026")).
  const sortKey = (d: string) => d.split("/").reverse().join("");
  seen.sort((a, b) => sortKey(a).localeCompare(sortKey(b)));
  const first = seen[0];
  const last = seen[seen.length - 1];
  if (first === last) return first;
  const sameYear = first.slice(6) === last.slice(6);
  return `${sameYear ? first.slice(0, 5) : first} → ${last}`;
}

// buildZaloMessageForTMDTShop gộp MỘT SHOP (mọi ngày trong đợt vừa chạy)
// thành một tin — đơn vị gom do groupKeyFor quyết định, xem
// lib/zaloGrouping.ts.
//
// Không dùng chung assembleOrderMessage/buildZaloMessageForPO được, cùng
// lý do như JIT: mỗi dòng ở đây đã là số liệu ĐÃ GỘP của cả một nhóm
// (shop, ngày) chứ không phải một đơn, nên "po đại diện" của
// assembleOrderMessage sẽ vô nghĩa và link tra cứu đơn cũng không tồn tại
// cho đơn sàn. Cũng không có sai giá / khuyến mãi: nhánh TMĐT lấy giá
// thẳng từ API sàn, không đối soát với bảng giá nội bộ.
//
// Tiền là TRƯỚC VAT — đúng bằng tổng cột Z (Thành tiền) vừa ghi vào
// dondathang.xlsx, để người nhận đối chiếu thẳng với file thay vì phải
// tự quy đổi 8%.
export function buildZaloMessageForTMDTShop(
  rows: OrderRow[],
  processedAt: string,
): string {
  if (rows.length === 0) return "";
  const first = rows[0];

  const shop = tmdtShopFromGroupKey(first.sourceId) || (first.po.split(" · ")[0] ?? "");

  // Một shop gần như luôn nằm trên đúng một sàn, nhưng nếu không thì tiêu
  // đề phải lùi về "TMĐT" chứ không nhận vơ sàn của dòng đầu tiên.
  const systems = [...new Set(rows.map((r) => r.system).filter(Boolean))];
  const heading = systems.length === 1 ? systems[0] : "TMĐT";

  const warehouses = [...new Set(rows.map((r) => r.shipTo).filter(Boolean))];
  const dateSpan = formatTMDTDateSpan(rows.map((r) => r.entryDate));

  const totalMoney = rows.reduce((sum, r) => sum + (Number(r.donGia) || 0), 0);
  const totalOrders = rows.reduce((sum, r) => sum + (r.totalOrders || 0), 0);
  const totalQty = rows.reduce((sum, r) => sum + (r.totalQty || 0), 0);
  const skuSet = new Set<string>();
  // `?? []`: Go trả null cho slice chưa gán (đã kiểm: `"skus":null`),
  // đúng lý do mọi chỗ khác trong file này cũng viết `?? []`.
  for (const r of rows) for (const sku of r.skus ?? []) skuSet.add(sku);
  const hasNA = rows.some((r) => r.statusKind === "warning");

  const header = `**🔔 ĐƠN HÀNG ${heading}**\n${DIVIDER}`;
  const identityLine = [
    `🏪 {orange:${shop}}`,
    dateSpan && `🗓️ ${dateSpan}`,
    warehouses.length > 0 && `📍 ${warehouses.join(" + ")}`,
  ]
    .filter(Boolean)
    .join(" · ");
  const countsLine = `🎫 Tổng số đơn: **${totalOrders} đơn** · 🏷️ Tổng số mã hàng: **${skuSet.size} mã**`;
  const totalsLine = `💰 **${formatMoney(totalMoney)}đ** · 📦 ${totalQty} sản phẩm`;

  const paragraphs = [
    [header, identityLine].join("\n"),
    [countsLine, totalsLine].join("\n"),
  ];
  if (hasNA) {
    paragraphs.push(
      '⚠️ {red:Còn mã chưa khai báo (#N/A)} — kiểm lại sheet "data shop" trước khi đẩy lên MISA.',
    );
  }
  if (processedAt) paragraphs.push(`⏱️ Xử lý lúc ${processedAt}`);

  return paragraphs.join("\n\n");
}
