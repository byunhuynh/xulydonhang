# Thiết kế: RealProcessor cho Winmart (Phase 2b-4 của dự án refactor)

## Bối cảnh

Phase 2b-1 (Lotte, 60/60), 2b-2 (Satra, 36/36), 2b-3 (BigC, 29/29) đã hoàn tất
và đóng. Phase 2b-4 làm **Winmart** — nhà cung cấp có 16 file mẫu thật, chưa
tăng từ lúc lập lộ trình ban đầu.

### Kế thừa từ các phase trước

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ" của
toàn Phase 2b. Cùng hạ tầng dùng chung: `excelwriter.Row`/`WriteOrderRows`,
`pricing.Index`/`FetchIndex`, và (từ BigC's Task 0) `processor_shared.go`'s
`regionInfo`/`closeEnough`/`buildPromoBonusRow`/`buildInvoiceBonusRow`/
`coopDebtDays`/`stripBlankLines`/`xPlus1Pattern`.

### Bài học từ review cuối của BigC — áp dụng ngay từ đầu

1. **Liệt kê từng khối trong hàm Python trước khi viết plan.** BigC's review
   cuối phát hiện khối khuyến mãi cấp hóa đơn ("Hóa Đơn") có trong spec
   nhưng bị rớt khỏi plan's pseudo-code, sống sót qua cả 9 task + 9 review
   vì mỗi bước chỉ đối chiếu đúng với plan (đã sai từ đầu). Mục "Khối trong
   `write_to_dondathang_winmart`" dưới đây liệt kê **đầy đủ** từng khối kèm
   dòng Python, để plan không thể vô tình bỏ sót cái nào.
2. Khi gặp lỗi Go/Python cùng lớp cơ chế (ví dụ CR/LF, Unicode whitespace)
   đã từng gặp ở vendor trước, kiểm tra xem có ảnh hưởng chéo vendor không
   trước khi khoanh vùng sửa quá hẹp.
3. Harness sinh fixture không nên đụng trực tiếp vào `dondathang.xlsx` sống
   nếu tránh được — Winmart's harness sẽ theo đúng protocol backup/restore
   đã dùng ổn định qua 3 phase trước (không đổi kiến trúc harness lần này,
   nhưng cẩn trọng với lock file thoáng qua như BigC từng gặp).

## Dữ liệu đối chiếu (golden corpus)

**16 file PDF Winmart thật** tại `đơn hàng/08-2026/*.pdf`, nhận diện qua
`identify_vendor`'s Winmart pattern (`xulydonhang.py:121-122`):
`re.search(r"Nhà cung cấp \(Supplier\): 0002011398", cleaned_text)` — một
pattern duy nhất, không có `or`, không phân biệt hoa/thường bị bỏ qua (Python
không dùng `re.IGNORECASE` ở đây, nhưng chuỗi tra cứu vốn cố định).

Ví dụ file: `4194002858.pdf` (1 trang), `4194003872.pdf` (2 trang),
`4194159265.pdf` (2 trang), `4194187081.pdf` (1 trang), `4901307932.pdf`
(2 trang), `4901307989.pdf` (1 trang).

**Xác nhận quan trọng: Winmart là "1 trang = 1 đơn"** — giống Coop/Lotte/
Satra, **khác hẳn BigC** (không có mô hình "trang 0 = bảng giá gốc + N
trang store"). Xác nhận qua vòng lặp trang của `process_file`
(`xulydonhang.py:7222`): mỗi trang được `identify_vendor` riêng, và nhánh
Winmart (`:8984-9089` + phần ghi, `:4154-4523`) xử lý độc lập từng trang,
chỉ cắt trang hiện tại ra file riêng để upload nếu file có nhiều trang
(`cat_trang_hien_tai`, cùng helper Coop/Lotte/Satra dùng) — không có trạng
thái xuyên trang nào cần thiết kế đặc biệt như BigC.

**Vị trí thật trong `identify_vendor`'s thứ tự kiểm tra** (đọc trực tiếp
toàn bộ hàm, không suy đoán): Coop → BigC → Lotte → Satra (2 dạng pattern)
→ Emart → Kingfood → CN-HCM → **Winmart** → SHOPEE-CHOICE → ... Vì Emart/
Kingfood/CN-HCM **chưa được port** sang Go, case Winmart trong Go's
`vendor.Identify` chỉ cần **nối sau Satra** (không cần chèn giữa như BigC
từng phải chèn giữa Coop và Lotte) — thứ tự tương đối vẫn đúng vì không có
gì ở giữa Satra và Winmart đã tồn tại trong Go.

## Mã khách hàng: tái dùng thẳng hàm fuzzy-match của Satra

**Khác BigC** (bảng tra cứu cứng), Winmart dùng **đúng hàm** Satra đã có:
`ProcessHandler.laymakhachhang_satra(diachi, "WINMART")` (gọi tại
`xulydonhang.py:9039` — số dòng tham chiếu tại thời điểm nghiên cứu, xác
nhận lại khi viết plan). Bên Go, điều này có nghĩa: **không cần code Go
mới cho phần tra mã khách hàng** — tái dùng thẳng
`productdata.Store.GetCustomerCodeByFuzzyAddress("WINMART", diachi)` đã
build từ Satra (Phase 2b-2), generic theo hệ thống truyền vào.

**Lưu ý khác biệt với `diachigiaohang`:** Winmart trích 2 địa chỉ riêng biệt
từ cùng trang PDF — `diachigiaohang` (địa chỉ kho + giao hàng literal, ghi
vào cột E của Excel) và `diachi` (một khối text khác, quét từ dòng chứa
"tổng hợp"+"wincommerce" đến "mst"/"địa chỉ giao hàng", dùng LÀM INPUT cho
fuzzy-match) — **hai khối hoàn toàn khác nhau, không được nhầm lẫn khi
port**. `diachigiaohang` không đi qua fuzzy-match; `diachi` không được ghi
trực tiếp vào cột nào.

## Khối trong `write_to_dondathang_winmart` (`xulydonhang.py:4154-4523`)

**Lưu ý quan trọng: hàm này được DÙNG CHUNG cho cả Winmart và BC Mart**
(gọi tại `:8462` cho BC Mart, `:9061`-vùng cho Winmart — số dòng cần xác
nhận lại khi viết plan) — chỉ rẽ nhánh nội bộ theo `vendor == 'WINMART'`
vs `'BC MART'` ở đúng 1 chỗ (công thức `giahoadon`, xem bảng dưới). BC Mart
**ngoài phạm vi** phase này (chưa có trong lộ trình 7 vendor), nhưng port
hàm Go tương ứng nên giữ khả năng mở rộng cho BC Mart sau này nếu dễ làm mà
không tốn thêm công sức đáng kể — không bắt buộc.

| Khối | Dòng | Tóm tắt | Khả năng tái dùng |
|---|---|---|---|
| Dòng header (metadata đơn, không sản phẩm) | 4212-4231 | Ghi A/B/C/D/G/L/V/AE/AJ/AM/U/Z/S/T/X/Y/E; `AV`=`songayno_MT` (dùng `coopDebtDays` đã có); vùng miền theo `makhachhang` (MB→HN, khác→LA, riêng `MN_MT_WIN1326`→DN) | Shell logic dùng `excelwriter.Row`/`WriteOrderRows` có sẵn; logic vùng miền là `winmartRegionInfo` mới (đã chốt: hàm riêng, không mở rộng `regionInfo` dùng chung) |
| **Trường hợp giá-0 "giao rời"** | 4251-4255 | `if dongia==0 and current_row-2>=9`: đánh dấu AO/AP vào **2 dòng TRƯỚC ĐÓ** (không ghi dòng riêng cho item giá-0), rồi `continue`. Điều kiện `current_row-2>=9` là số dòng TUYỆT ĐỐI trong sheet, không tương đối theo đơn hàng — có thể đọc nhầm sang đơn khác nếu item giá-0 là sản phẩm đầu/thứ 2 của đơn | **Đã chốt thiết kế**: Go chỉ đánh dấu trong phạm vi rows đã tích lũy của ĐƠN HÀNG HIỆN TẠI; nếu không có dòng nào trước đó trong đơn này, bỏ qua an toàn thay vì đọc/ghi chéo sang đơn khác — cải tiến chủ động theo chính sách Phase 2b |
| Vòng lặp sản phẩm chính (giá/AU/AT/Z/X, khớp CTKM đơn) | 4234-4404 | Trường SKU/qty/giá chuẩn, tích lũy AT (khối lượng)/**AU (số kiện — Winmart CÓ ghi, khác BigC**), khớp 1 CTKM duy nhất (không có `split('|')`), tô đỏ+comment khi sai giá | Pattern tô đỏ/comment giống Coop/Satra/BigC đã có; vòng lặp khớp-1-CTKM là dạng đơn giản hơn Coop/Satra, gần giống BigC's dạng đơn |
| Dòng khuyến mãi tặng kèm theo sản phẩm | 4406-4460 | `check_value_in_sanpham`, logic chia số lượng theo "X+1", che barcode qua `laycachbo_khuyenmai`, ghi dòng thứ 2 cho hàng tặng. Fallback không che: `'KM Giao Rời - Không Che Barcode'` (`:4429`) | Cùng dạng với các vendor khác; giá trị field riêng của Winmart |
| **Dòng khuyến mãi cấp hóa đơn ("Hóa Đơn")** | 4465-4506 | `find_all_promotions_by_sku_and_time("Hóa Đơn", entry_date, vendor)`, `soluongkm = floor(tongtien / tachtien_khuyenmai(...))`, ghi 1 dòng bonus. Fallback không che: `'KM Bó Kèm - Che Barcode'` (`:4502`) | **BẮT BUỘC PORT NGAY TỪ ĐẦU** (bài học BigC) — cùng dạng Coop/Satra/BigC's khối invoice-level, tái dùng được phần lớn logic đã viết cho BigC's khối tương tự |
| Ghi tổng khối lượng vào ô L của dòng header | 4508 | `sheet[f"L{start_row}"] = ...` | Khớp thẳng `excelwriter.WriteOrderRows`'s tham số `headerDescription` |
| Lưu + log | 4509-4523 | `wb.save`, log timing/summary, `return saigia` | Không port (đã xác nhận Go không port upload Drive ở bất kỳ vendor nào) |

## Phạm vi

### Làm thật

- Trích PO/ngày/địa chỉ từ trang PDF (`xulydonhang.py:8984-9089` — số dòng
  cần xác nhận lại chính xác khi viết plan, đây là nghiên cứu ban đầu):
  - `entry_date`: dòng sau `"Ngày đặt hàng (PO date)"`, thay `.`→`/`.
  - `po_number`: dòng sau `"Số đơn hàng (PO No.)"`.
  - `cancel_date`: dòng sau `"Ngày giao (Delivery Date)"`, thay `.`→`/`.
  - `diachigiaohang` (ghi cột E): mã kho + địa chỉ sau
    `"Địa chỉ giao hàng (Delivery Address)"` đến trước
    `"Thông tin đơn hàng (Information)"`, lọc bỏ dòng trùng chứa `"WM+"`.
  - `diachi` (input fuzzy-match, KHÔNG ghi cột nào): quét từ dòng chứa cả
    "tổng hợp"+"wincommerce" (hoặc chỉ "wincommerce") đến trước
    "mst"/"địa chỉ giao hàng".
  - `ghichu`: giữa "Ghi chú" và chuỗi literal nhà cung cấp, nối bằng
    khoảng trắng.
- Trích danh sách sản phẩm — hàm `trichxuatsanpham_winmart`
  (`xulydonhang.py:6704-6735`, `re.VERBOSE`, chạy trên text đã nối nhiều
  trang) — trả về `{Barcode, OU Qty, Total Price}`, không có field giá đơn
  vị riêng.
- Mã khách hàng: tái dùng `productdata.GetCustomerCodeByFuzzyAddress`
  ("WINMART", `diachi`) — không code Go mới.
- `winmartRegionInfo`: hàm vùng miền riêng (MB→HN/MT_MB, khác→LA/MT_MN,
  riêng `MN_MT_WIN1326`→DN — giá trị chính xác cần đối chiếu lại khi viết
  plan).
- Khối "giao rời giá-0": theo thiết kế đã chốt ở trên (chỉ trong phạm vi
  đơn hiện tại).
- Khối khuyến mãi cấp hóa đơn "Hóa Đơn": port đầy đủ ngay từ đầu.
- Mở rộng `vendor.Identify` — nối case Winmart sau Satra.
- Mở rộng dispatch của `RealProcessor.Process` — Winmart là "1 trang = 1
  đơn" nên dùng lại đúng pattern per-page dispatch hiện có (như Lotte/Satra),
  không cần pre-check + document-handler riêng như BigC.
- File mới `GO/internal/processing/winmart/` (package trích xuất thuần) +
  `winmart_processor.go` (dispatch + row builder) + test tương ứng — theo
  đúng pattern đã thiết lập từ Satra (mỗi vendor file riêng ngay từ đầu).

### Không làm (ngoài phạm vi Phase 2b-4)

- BC Mart — dù dùng chung hàm Python `write_to_dondathang_winmart`, BC Mart
  không nằm trong lộ trình 7 vendor đã thống nhất; không chủ động port
  nhánh `vendor == 'BC MART'`.
- Emart, FujiMart, Kingfood — sub-project riêng sau.
- Upload PDF lên Google Drive — không port ở bất kỳ vendor nào.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go              # + case Winmart, NỐI SAU Satra (không chèn
                              # giữa — không có vendor nào giữa Satra và
                              # Winmart trong thứ tự Python thật đã tồn
                              # tại trong Go)
  winmart/
    extract.go                # ParsePOAndDates, ParseAddresses (2 địa chỉ
                              # riêng biệt — xem mục trên), ExtractProducts
  winmart_processor.go          # processWinmartSegment — mirror
                              # processSatraSegment/processLotteSegment's
                              # cấu trúc per-page, KHÔNG mirror BigC's
                              # per-document. Có bộ dựng row riêng (không
                              # tái dùng buildPromoBonusRow/buildInvoiceBonusRow
                              # trực tiếp — cần xác nhận lại khi viết plan
                              # liệu Winmart's promo-loop shape đơn giản
                              # hơn có khớp đủ để tái dùng buildPromoBonusRow,
                              # khác BigC's trường hợp không khớp hẳn)
  winmart_processor_test.go
  winmart_golden_test.go        # đối chiếu 16 fixture JSON

GO/internal/processing/winmart/testdata/
  fixtures/*.json
  generate_fixtures.py
```

### Ánh xạ trạng thái OrderRow

Tái dùng nguyên `StatusDone/StatusWarning/StatusFailed`.

## Dữ liệu ngoài & phụ thuộc mạng

- **`data.xlsx`**: sheet `MaKH` cần có dòng hệ thống `WINMART` với địa chỉ
  mẫu cho test (theo đúng pattern Satra's Task 2 đã làm) — kiểm tra xem đã
  có sẵn hay cần bổ sung khi viết plan.
- **`settings.ini`**: đã xác nhận có sẵn `WINMART = 1817734506` (gid) và
  2 entry hiển thị vùng miền (`MNWINMART`/`MBWINMART`) — không cần sửa.
- **Google Drive upload**: không gọi ở phase này.

## Testing

- Go unit test cho từng hàm trích xuất thuần trong `winmart/` (TDD).
- `winmart_processor_test.go`: test riêng cho khối giá-0 "giao rời" (cả 2
  nhánh: có dòng trước trong đơn để đánh dấu, và không có — bỏ qua an
  toàn) và khối khuyến mãi cấp hóa đơn.
- `winmart_golden_test.go`: đối chiếu toàn bộ 16 fixture, dùng
  `knownDivergences_Winmart` theo đúng key format `sourcePDF:row:col`.

## Rủi ro / lưu ý

- **Hàm ghi được dùng chung với BC Mart** — cẩn thận khi port để không vô
  tình giả định field nào đó luôn có giá trị theo cách chỉ đúng với
  Winmart (ví dụ công thức `giahoadon` rẽ nhánh theo `vendor` ở
  `xulydonhang.py:4299-4302` — phải port cả 2 nhánh dù chỉ dùng nhánh
  WINMART, để code Go không âm thầm giả định sai nếu BC Mart được thêm
  sau này — hoặc ít nhất ghi rõ comment chỉ port nhánh WINMART).
- **Khối giá-0 "giao rời"**: thiết kế đã chốt (chỉ đánh dấu trong phạm vi
  đơn hiện tại) là quyết định chủ động, cần đối chiếu với dữ liệu thật khi
  chạy golden fixture — nếu không file mẫu nào trong 16 file thật kích
  hoạt trường hợp "sản phẩm đầu tiên là giá-0", quyết định này sẽ không có
  bằng chứng thực nghiệm trực tiếp; viết unit test tổng hợp (synthetic) để
  bù đắp, theo đúng cách BigC's Hóa Đơn bonus row đã làm.
- **Chưa xác nhận liệu Winmart's promo/bonus-row shape có đủ giống Coop để
  tái dùng thẳng `buildPromoBonusRow`/`buildInvoiceBonusRow`** hay cần bộ
  dựng riêng như BigC — cần đọc kỹ `write_to_dondathang_winmart`'s đoạn
  4406-4460 và so sánh trực tiếp với `buildPromoBonusRow`'s logic khi viết
  plan, tương tự cách BigC's spec ban đầu đã nhận định sai (rồi phải sửa
  lại lúc viết plan) — không giả định trước.
- 16 file mẫu hiện có — nếu tương lai có thêm, hành vi kỳ vọng vẫn là dòng
  "Thất bại" rõ ràng khi không parse được.
