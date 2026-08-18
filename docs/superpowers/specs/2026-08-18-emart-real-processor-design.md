# Thiết kế: RealProcessor cho Emart (Phase 2b-5 của dự án refactor)

## Bối cảnh

Phase 2b-1 (Lotte, 60/60), 2b-2 (Satra, 36/36), 2b-3 (BigC, 29/29), 2b-4
(Winmart, 12/12 fixture có coverage) đã hoàn tất và đóng. Phase 2b-5 làm
**Emart**.

**Điều chỉnh số liệu:** lộ trình ban đầu ghi nhận Emart có ~9 file mẫu. Chạy
lại đúng logic `identify_vendor` (`xulydonhang.py:111-112`) trên toàn bộ
`đơn hàng/08-2026/` cho ra **17 file PDF Emart thật**, không phải 9 — số
liệu trong lộ trình cần cập nhật.

### Kế thừa từ các phase trước

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ" của
toàn Phase 2b. Cùng hạ tầng dùng chung: `excelwriter.Row`/`WriteOrderRows`,
`pricing.Index`/`FetchIndex`, và `processor_shared.go`'s
`regionInfo`/`closeEnough`/`buildPromoBonusRow`/`buildInvoiceBonusRow`/
`coopDebtDays`/`stripBlankLines`/`xPlus1Pattern`.

### Bài học từ Winmart's review cuối — áp dụng ngay từ đầu

1. **Liệt kê từng khối trong hàm Python trước khi viết plan** (đã áp dụng
   dưới, xem bảng "Khối trong `write_to_dondathang_emart`") — bài học từ
   BigC, nhắc lại vì vẫn còn giá trị.
2. **Liệt kê luôn từng yêu cầu test trong mục "Testing" của spec như một
   checklist riêng**, không chỉ các khối tính năng — Winmart's review cuối
   phát hiện yêu cầu test 2-nhánh cho khối giá-0 bị rớt khỏi plan dù thiết
   kế (feature) được port đúng; mục Testing dưới đây liệt kê rõ từng test
   bắt buộc.
3. Khi gặp guard dựa trên số dòng/tuyệt đối tương tự cơ chế "current_row"
   của Winmart, tự tay truy vết lại phép tăng dòng của Python thay vì tin
   theo trực giác — Winmart's guard ban đầu sai lệch 1 đơn vị, chỉ phát
   hiện được nhờ truy vết thủ công ở review cuối cùng.
4. Xét khả năng hợp nhất `extractWinmartPageText`-kiểu Y-tracking với
   `reconstructLinesFromContent` của Coop nếu Emart's PDF cũng gặp cùng lớp
   lỗi thư viện `ledongthuc/pdf` (BT/T*-only line break) — kiểm tra sớm khi
   viết plan/chạy fixture thay vì đợi đến review cuối.

## Dữ liệu đối chiếu (golden corpus)

**17 file PDF Emart thật** tại `đơn hàng/08-2026/*.pdf`:
```
4501866956.PDF   4501866958.PDF   4501873464.PDF   4501873471.PDF
4501873478.PDF   4501875697.PDF   4501875698.PDF   4501875699.PDF
4501878295.PDF   4501880037.PDF   4501880038.PDF   4501880119.PDF
4501880122.PDF   4501880895.PDF   4501880904.PDF   4501880907.PDF
4501881986.PDF
```
Tất cả xác nhận **1 trang/file** (kiểm tra trực tiếp `4501866956.PDF`).

**Nhận diện** (`identify_vendor`, `xulydonhang.py:111-112`):
```python
if re.search(r"CONG TY TNHH TMDV XNK HA THANH \(101017\)", cleaned_text) or re.search(r"THISO RETAIL COMPANY LIMITED", cleaned_text):
    return "Emart"
```
Kiểm tra trực tiếp trên `4501866956.PDF`: chỉ pattern thứ 2
(`THISO RETAIL COMPANY LIMITED`, tên hãng phát hành PO thật) khớp — pattern
ASCII không dấu đầu tiên **không bao giờ khớp** vì text PDF thật chứa dạng
có dấu `CÔNG TY TNHH TMDV XNK HÀ THÀNH  (101017)`. **Quyết định: vẫn mirror
cả 2 regex** trong Go (kể cả pattern thực tế chết) để trung thực với
Python, có comment ghi rõ pattern nào mới thực sự hoạt động trên dữ liệu
thật — không âm thầm bỏ pattern chết, tránh rủi ro một PDF tương lai nào
đó lại dùng đúng dạng ASCII.

**Xác nhận: Emart là "1 trang = 1 đơn"** — cùng nhóm Coop/Lotte/Satra/
Winmart, khác BigC. Xác nhận qua nhánh `elif vendor == "Emart":` trong
`process_file` (`xulydonhang.py:9314-9384`): mọi trích xuất chỉ thao tác
trên biến `text` của trang hiện tại, không tích lũy trạng thái xuyên trang;
17/17 file mẫu thật đều đúng 1 trang.

**Vị trí trong `identify_vendor`'s thứ tự kiểm tra** (đọc trực tiếp toàn bộ
hàm, `xulydonhang.py:90-179`): Coop → BigC → Lotte → Satra (2 dạng) →
**Emart** → Kingfood → CN-HCM → Winmart → ... Vì Winmart đã port và nối
ngay sau Satra trong Go hiện tại (không có Kingfood/CN-HCM/Emart nào ở
giữa từng tồn tại), case Emart trong Go's `vendor.Identify` phải **chèn
giữa Satra và Winmart** (không phải nối cuối) — khác với Winmart's case
(chỉ cần nối sau Satra vì lúc đó chưa có gì ở giữa).

## Mã khách hàng: hằng số cứng, không tra cứu

**Khác cả Satra (fuzzy-match) lẫn BigC (bảng tra cứu theo tên store)**:
Emart's `makhachhang` **không bao giờ được trích từ PDF** — luôn là literal
`"MN_MT_KH0032"`, truyền thẳng từ call site trong `process_file`
(`xulydonhang.py:9363`). Không có hàm Go mới nào cần cho phần này —
`emartProcessSegment` chỉ dùng hằng số Go tương ứng.

Vì `makhachhang` luôn cố định, nhánh `makhachhang[:2] == "MB"`
(`xulydonhang.py:5003-5009`) **không bao giờ đạt tới trên thực tế** — mọi
đơn Emart thật đều resolve `kho="LA_KHO2026"`, `khuvuc="MT_MN"`,
`mien="LA"`. Lưu ý: `LA_KHO2026`, **không phải** `LA_TP` mà `regionInfo()`
dùng chung trả về cho nhánh non-MB — cùng divergence đã xử lý cho Winmart
(`winmartRegionInfo`) và BigC (`bigcRegionInfo`).

**Quyết định:** vẫn viết `emartRegionInfo(customerCode string) (region,
statCode, warehouse string)` như một hàm đầy đủ 2 nhánh (dù nhánh MB không
đạt tới với input thật hiện tại) — theo đúng pattern Winmart, để nhất
quán kiến trúc và không giả định "hardcode luôn đúng mãi mãi" nếu
`makhachhang`'s nguồn gốc thay đổi trong tương lai (ví dụ nếu Emart sau
này cũng được thêm fuzzy-match). Chi phí thêm gần như bằng 0.

## Trích xuất sản phẩm — bẫy ngữ nghĩa cần lưu ý khi port

Hàm `laydanhsanpham_emart` (`xulydonhang.py:6614-6644`):

```python
def laydanhsanpham_emart(text):
    product_pattern = re.finditer(r"""
        (?P<article_code>\d{7})\s*
        (?P<barcode>\d{12,13})\s*
        \s*(?P<description>.+?)\s+
        (?P<unit>[A-Z]{2,})\s+
        \s*(?P<qty_in_box>\d+)\s+
        \s*(?P<quantity>\d+)\s+
        \s*(?P<purchase_price>[\d\.,]+)
    """, text.strip(), re.VERBOSE | re.DOTALL)

    results = []
    for match in product_pattern:
        purchase_price = match.group("purchase_price").replace(".", "")
        purchase_price_value = float(purchase_price.replace(",", "."))
        if purchase_price_value == 0:
            continue
        results.append({
            "Barcode": match.group("barcode"),
            "OU Qty": int(match.group("quantity")),
            "Total Price": purchase_price
        })
    return results
```

**BẪY QUAN TRỌNG:** cột thật trong bảng sản phẩm PDF là `Article Code /
Unit Barcode / Description / PO Unit Qty. in Box / PO Qty. / Pur.
Price(-VAT) / Amount(-VAT) / Free PO`. Group `purchase_price` bắt
**`Pur. Price` — đơn giá/1 đơn vị** (xác nhận trực tiếp trên
`4501866956.PDF`: bắt được `26.950`, không phải cột `Amount` tổng dòng
`1.293.600`). Dict trả về **đặt tên field là `"Total Price"` nhưng giá trị
thực chất là đơn giá**, và `write_to_dondathang_emart` dùng thẳng làm
`giahoadon` (`:5095`) **không chia cho quantity**.

**Đây là khác biệt cốt lõi với Winmart** — Winmart's field cùng tên
`"Total Price"` thật sự là tổng tiền dòng, phải chia
(`invoicePrice := totalPrice / ouQty`, `winmart_processor.go:203`). Nếu
port Emart theo kiểu copy pattern Winmart mà không đọc kỹ, sẽ vô tình chia
nhầm đơn giá Emart cho quantity — **plan phải ghi rõ ràng, không chia**,
và Go's field nên đặt tên phản ánh đúng bản chất (ví dụ `UnitPrice` thay
vì `TotalPrice`) để tránh nhầm lẫn tương lai, dù dict key gốc Python dùng
`"Total Price"`.

**Bỏ hàng giá-0 xảy ra ngay tại đây** (`purchase_price_value == 0: continue`,
dòng 6627-6628) — đơn giản hơn Winmart nhiều: sản phẩm giá-0 chỉ đơn thuần
không xuất hiện trong `results`, **không có logic đánh dấu AO/AP vào dòng
trước đó nào cả**. Go's `ExtractProducts` chỉ cần bỏ qua item giá-0 tại
bước parse, không cần hàm kiểu `winmartZeroPriceSkip`.

## Khối trong `write_to_dondathang_emart` (`xulydonhang.py:4974-5330`)

| Khối | Dòng | Tóm tắt | Khả năng tái dùng |
|---|---|---|---|
| Ánh xạ tên store → mã ngắn | 4990-4996 | Dict cứng 3 entry: `"EMART GO VAP"→"PVT"`, `"EMART PHI"→"PHI"`, `"EMART SALA"→"SALA"`; `mapping.get(congtrinh, congtrinh)` — fallback giữ nguyên text gốc nếu không khớp | Hàm Go mới, nhỏ — chưa vendor nào có logic tương tự |
| Dòng header (metadata đơn) | 4998-5054 | Ghi A/AV/B/C/D/G/L/V/AE/AJ/AM/U/Z/S/T/X/Y/E; vùng miền theo `makhachhang` (xem mục trên); cột **K** ghi 1-trong-3 tên đầy đủ store (`if/elif` cứng theo mã ngắn, `:5046-5051`) hoặc để trống nếu không khớp; cột **AN ghi cứng literal `'PVT'`** bất kể store thật (`:5054`) | Shell dùng `excelwriter.Row`; vùng miền dùng `emartRegionInfo`; cột K/AN — xem "Rủi ro" bên dưới |
| Vòng lặp sản phẩm chính | 5059-5199 | SKU/qty/giá chuẩn (giá không chia, xem mục trên); AT (khối lượng) có ghi; **không ghi AU** (khác Winmart — cần xác nhận lại tại plan bằng cách grep `AU{current_row}` trong đoạn này, hiện tại đọc source không thấy có); tô đỏ+comment khi sai giá (pattern giống mọi vendor khác) | Pattern tô đỏ/comment tái dùng logic đã có; vòng lặp khớp giá dạng đơn giản (không multi-CTKM ở bước NÀY — multi-CTKM chỉ ở khối bonus bên dưới) |
| **Khối khuyến mãi tặng kèm theo sản phẩm (multi-CTKM)** | 5203-5271 | `khuyenmai.split('|')` — multi-CTKM giống Coop, vòng `for i, hangkm in enumerate(...)`; check `X+1` qua regex, chia qty; `i==0` ghi AO/AP vào dòng sản phẩm CHÍNH + dòng bonus; `i>0` chỉ ghi dòng bonus tiếp theo. **Fallback không-ngoặc: `'KM Rời - Không Che Barcode'`** (`:5230`, `:5240`) — chuỗi thứ 3, khác cả Coop (`'KM Bó Kèm - Che Barcode'`) lẫn Winmart (`'KM Giao Rời - Không Che Barcode'`). Fallback này KHÔNG ghi AP (giống Winmart, khác Coop) | Cấu trúc đủ giống Coop để tái dùng `buildPromoBonusRow`, override call site fallback text — đúng pattern đã dùng cho Lotte và Winmart, lần thứ 3 |
| **Khối khuyến mãi cấp hóa đơn ("Hóa Đơn")** | 5274-5316 | `find_all_promotions_by_sku_and_time("Hóa Đơn", ...)`, `soluongkm = floor(tongtien / tachtien_khuyenmai(...))`, ghi `Q=kiemtra[0]` (chỉ SKU đầu, không nối chuỗi — giống Winmart, khác `buildInvoiceBonusRow` mặc định). Fallback: `'KM Bó Kèm - Che Barcode'` (`:5312`, khớp mặc định chung). **`kiemtra[0]` không có guard rỗng** (`:5290`) — crash tiềm ẩn trong Python nếu `kmhoadon` không map ra SKU nào | `buildInvoiceBonusRow` đã có guard `len(skus)==0` sẵn — Go dùng thẳng guard này thay vì tái tạo nguy cơ crash của Python; cần override "chỉ SKU đầu" giống Winmart đã làm |
| Ghi tổng khối lượng vào ô L của dòng header | 5317 | `sheet[f"L{start_row}"] = f'{diengiai} (Tổng trọng lượng: ...)'` | Khớp thẳng `excelwriter.WriteOrderRows`'s `headerDescription` |
| Lưu + log | 5318-5330 | `wb.save`, log timing/summary, `return saigia` | Không port (nhất quán mọi vendor trước) |

## Phạm vi

### Làm thật

- Trích PO/ngày/store — hoàn toàn **inline trong `process_file`'s nhánh
  Emart** (`xulydonhang.py:9314-9340`), không có hàm `_emart`-suffix riêng
  cho phần này (khác Winmart's `ParseOrderInfo`) — Go vẫn nên gom vào 1
  hàm package-level `emart.ParseOrderInfo(text string)` theo đúng convention
  đã lập cho mọi vendor khác, dù Python không tách hàm riêng:
  - `po_number`: `re.search(r"PO No\.\s*\n\s*:? ?([^\n]+)", text)` (`:9316`).
  - `entry_date`: `re.search(r"Order By / Date\s*\n\s*:? ?([^\n]+)", text)`,
    lấy `[:10]`, thay `.`→`/` (`:9322-9326`).
  - `cancel_date`: cùng dạng, marker `"Delivery Date"` (`:9327-9331`).
  - `tenstore` (ghi cột E): `re.search(r"^Delivery to :\s*(.+)", text,
    re.MULTILINE)`, rồi `.split("   ")[0]` — tách trên 3 khoảng trắng liên
    tiếp (`:9333-9338`).
- Isolate vùng bảng sản phẩm trước khi trích: `re.search(r"Article
  Code\s*(.*?)\s*Total Amount\(without VAT\) :", text, re.DOTALL)`
  (`:9339-9340`) — thực hiện trước khi gọi hàm trích sản phẩm.
- Trích sản phẩm — `laydanhsanpham_emart` (`:6614-6644`), xem mục "bẫy
  ngữ nghĩa" ở trên. Trả về `{Barcode, OUQty, UnitPrice}` (đặt tên Go rõ
  ràng hơn dict key Python gốc).
- Ánh xạ tên store: hàm Go nhỏ mirror 2 bảng tra cứu (mã ngắn +
  tên đầy đủ cột K) tại `xulydonhang.py:4990-4996` và `:5046-5051`.
- `emartRegionInfo`: theo thiết kế đã chốt ở trên.
- Bỏ hàng giá-0: xử lý tại bước trích xuất (extraction), không cần logic
  đánh dấu dòng trước.
- Khối khuyến mãi tặng kèm theo sản phẩm: tái dùng `buildPromoBonusRow`,
  override fallback text thành `'KM Rời - Không Che Barcode'`, không ghi AP
  ở nhánh fallback.
- Khối khuyến mãi cấp hóa đơn: port đầy đủ, dùng `buildInvoiceBonusRow`'s
  guard rỗng sẵn có thay vì tái tạo crash risk của Python.
- Chèn `vendor.Identify`'s case Emart **giữa Satra và Winmart** (không
  phải nối cuối).
- Mở rộng dispatch của `RealProcessor.Process` — Emart "1 trang = 1 đơn"
  dùng lại per-page dispatch hiện có.
- File mới `GO/internal/processing/emart/` (package trích xuất thuần) +
  `emart_processor.go` (dispatch + row builder) + test tương ứng.

### Không làm (ngoài phạm vi Phase 2b-5)

- Cột **AN**: gap kiến trúc đã ghi nhận từ BigC/Winmart — Go chưa ghi cột
  AN cho **bất kỳ** vendor nào (`excelwriter.Row` chưa có field tương ứng).
  Emart's Python có bug tiềm ẩn tại đây (dòng header ghi cứng `AN='PVT'`
  bất kể store thật, trong khi dòng sản phẩm ghi đúng `congtrinh` đã map —
  khả năng là copy-paste bug, không phải chủ đích) — **không xử lý trong
  phase này**, chỉ ghi chú lại. Khi cột AN được port project-wide (việc
  chờ từ lâu), quyết định "port đúng bug" hay "sửa" cho Emart sẽ cần ra
  riêng lúc đó.
- Kingfood, FujiMart — sub-project riêng sau, theo đúng lộ trình.
- Upload PDF lên Google Drive — không port ở bất kỳ vendor nào.
- Cột G1 (`sheet["G1"] = STT_donhang`) — không vendor Go nào ghi hiện tại,
  giữ nguyên tiền lệ.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go              # + case Emart, CHÈN GIỮA Satra và Winmart
                              # (thứ tự Python thật: Satra → Emart →
                              # Kingfood → CN-HCM → Winmart; Kingfood/
                              # CN-HCM chưa port nên không ảnh hưởng thứ
                              # tự tương đối Emart-trước-Winmart)
  emart/
    extract.go                # ParseOrderInfo (PO/date/store, dù Python
                              # không tách hàm riêng — Go vẫn gom theo
                              # convention chung), ExtractProducts
  emart_processor.go           # processEmartSegment — mirror
                              # processSatraSegment/processWinmartSegment's
                              # cấu trúc per-page
  emart_processor_test.go
  emart_golden_test.go         # đối chiếu 17 fixture JSON

GO/internal/processing/emart/testdata/
  fixtures/*.json
  generate_fixtures.py
```

### Ánh xạ trạng thái OrderRow

Tái dùng nguyên `StatusDone/StatusWarning/StatusFailed`.

## Dữ liệu ngoài & phụ thuộc mạng

- **`data.xlsx`**: Emart không cần fuzzy-match customer-code (mã khách
  hàng luôn hardcode), nên **không cần dòng `MaKH` mới** cho Emart — khác
  Satra/Winmart's Task 2 đã từng cần bổ sung. Xác nhận lại khi viết plan
  nếu có phần khác của test cần dữ liệu `SanPham`/giá mẫu.
- **`settings.ini`**: cần xác nhận có sẵn entry `EMART` (gid cho sheet giá/
  CTKM) hay chưa khi viết plan — `find_price_by_sku`/
  `find_all_promotions_by_sku_and_time` được gọi với `vendor="EMART"`
  (`xulydonhang.py:5085`, `:5119`), ngụ ý cần sheet `"EMART"` tồn tại trong
  Google Sheet giá/CTKM, tương tự `"WINMART"`/`"SATRA"`/`"COOP"` đã có.
- **Google Drive upload**: không gọi ở phase này.

## Testing

Checklist bắt buộc (rút kinh nghiệm từ Winmart — liệt kê rõ từng test, không
chỉ nói chung chung "test đầy đủ"):

- Go unit test cho từng hàm trích xuất thuần trong `emart/` (TDD):
  `ParseOrderInfo`, `ExtractProducts` (bao gồm test riêng cho case giá-0
  bị lọc bỏ đúng, và case field "Total Price" thật ra là đơn giá — không
  bị chia nhầm).
- `emart_processor_test.go`:
  - Test ánh xạ tên store → mã ngắn + cột K (3 case named store + 1 case
    tên lạ/fallback).
  - Test `emartRegionInfo` (dù nhánh MB không đạt tới với input thật hiện
    tại, vẫn test cả 2 nhánh trực tiếp qua hàm thuần — không phụ thuộc PDF
    thật, theo đúng cách `TestWinmartRegionInfo` đã làm).
  - Test khối khuyến mãi tặng kèm — fallback không-ngoặc dùng đúng chuỗi
    `'KM Rời - Không Che Barcode'`, không ghi AP.
  - Test khối khuyến mãi cấp hóa đơn — chỉ SKU đầu (`kiemtra[0]`, không
    nối chuỗi), và case rỗng (không SKU nào khớp) không crash, không ghi
    dòng bonus.
- `emart_golden_test.go`: đối chiếu toàn bộ 17 fixture, dùng
  `knownDivergences_Emart` theo đúng key format `sourcePDF:row:col`.

## Rủi ro / lưu ý

- **Bẫy field "Total Price" = đơn giá, không phải tổng tiền** — rủi ro lớn
  nhất của phase này, dễ port sai nếu copy theo pattern Winmart mà không
  đọc kỹ giá trị regex thật bắt được trên PDF mẫu. Plan phải trích dẫn
  trực tiếp 1 giá trị mẫu thật (ví dụ `26.950` từ `4501866956.PDF`) để
  người viết code xác nhận trước khi implement.
- **Cột AN header ghi cứng `'PVT'`** — khả năng là bug Python, nhưng hiện
  tại ngoài phạm vi vì cột AN chưa được port cho vendor nào. Không tự ý
  "sửa" khi (nếu) port AN sau này mà không có quyết định rõ ràng — ghi rõ
  cả 2 khả năng (port đúng bug / sửa) trong ghi chú lúc đó.
- **`kiemtra[0]` không guard rỗng ở Python** — Go dùng guard sẵn có của
  `buildInvoiceBonusRow`, chủ động không tái tạo crash risk, nhất quán
  chính sách "không bắt buộc giữ bug cũ" của Phase 2b.
- **Chưa xác nhận cột AU có được Emart ghi hay không** — đọc source ban
  đầu không thấy dòng `AU{current_row}` nào trong vòng lặp sản phẩm chính
  (khác Winmart, giống BigC không ghi AU) — cần xác nhận lại bằng cách
  grep trực tiếp đoạn `5059-5199` khi viết plan, không giả định.
- 17 file mẫu hiện có — nếu tương lai có thêm, hành vi kỳ vọng vẫn là dòng
  "Thất bại" rõ ràng khi không parse được.
