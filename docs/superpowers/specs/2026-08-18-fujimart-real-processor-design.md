# Thiết kế: RealProcessor cho FujiMart (Phase 2b-6 của dự án refactor)

## Bối cảnh

Phase 2b-1 đến 2b-5 (Lotte, Satra, BigC, Winmart, Emart) đã hoàn tất và
đóng. Phase 2b-6 làm **FujiMart** — nhà cung cấp có **15 file mẫu thật
khả dụng ngay** (1 file ở thư mục sống `đơn hàng/08-2026/`, 14 file trong
kho lưu trữ `đơn hàng/mẫu đơn hàng/*/` — xem mục "Dữ liệu đối chiếu").

### Kế thừa từ các phase trước

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ".
Cùng hạ tầng dùng chung: `excelwriter.Row`/`WriteOrderRows`,
`pricing.Index`/`FetchIndex`, `processor_shared.go`'s
`regionInfo`/`closeEnough`/`buildPromoBonusRow`/`buildInvoiceBonusRow`/
`coopDebtDays`/`stripBlankLines`/`xPlus1Pattern`.

### Bài học từ Emart's close — áp dụng ngay từ đầu

1. **Đã thực hiện TRƯỚC KHI viết spec này**: chạy một PDF FujiMart thật
   qua đúng pipeline Go thật (`extractPageTexts`) trước khi thiết kế bất
   kỳ logic trích xuất nào — không chỉ dựa vào PyMuPDF/giả định layout từ
   Python. Kết quả xác nhận: (a) **không cần OCR** — tên chi nhánh nằm
   sẵn trong lớp text; (b) các chuỗi marker lỗi encoding (mojibake) Python
   dựa vào tái tạo **y hệt** trong Go; (c) một số cặp label/giá trị bị
   tách thành 2 dòng riêng trong Go dù cùng dòng trong Python (cùng lớp
   lỗi đã gặp ở Emart's `ParseOrderInfo`) — thiết kế trích xuất bên dưới
   đã tính đến điều này ngay từ đầu, dùng cách quét dòng (line-scan) chịu
   được cả 2 layout, không dùng regex-cùng-dòng ngây thơ.
2. **⚠️ Rủi ro kiến trúc đang hoạt động, không riêng FujiMart**: thư mục
   `đơn hàng/08-2026/` KHÔNG phải nguồn dữ liệu ổn định — một tiến trình
   thật, đang chạy song song (ứng dụng thật của người dùng) xử lý và tổ
   chức lại thư mục này liên tục. Xác nhận lại ngay lúc viết spec này:
   thư mục hiện chỉ còn 10 file (khác hẳn ~349 file ban đầu). Plan này
   sẽ áp dụng NGAY từ Task 5 (không đợi đến khi gặp sự cố như Emart) thiết
   kế copy các PDF thật tìm được vào thư mục testdata ổn định, git-tracked
   (`GO/internal/processing/fujimart/testdata/realpdfs/`) — cần xác nhận
   lại với chủ dự án trước khi commit dữ liệu khách hàng thật vào git
   (đã hỏi và được đồng ý làm vậy cho Emart; hỏi lại riêng cho FujiMart,
   không giả định cùng câu trả lời).
3. Nếu gặp guard dựa trên vị trí tuyệt đối/offset dòng (như FujiMart's
   entry_date extraction bên dưới), tự tay xác minh trực tiếp trên PDF
   thật bằng cả 2 pipeline (Python và Go) trước khi tin, không suy diễn
   từ đọc code một chiều.

## Dữ liệu đối chiếu (golden corpus)

**15 file PDF FujiMart thật khả dụng ngay** (tại thời điểm viết spec —
đây là ảnh chụp tức thời, thư mục sống có thể đổi):

`đơn hàng/08-2026/` (1 file):
```
103001302608001342.pdf
```

`đơn hàng/mẫu đơn hàng/*/` (14 file, đã đổi tên theo định dạng
`<ngày xử lý>_[FujiMart][entryDate][MB_MT_FUJI][cancelDate][PO gốc].pdf`):
```
04-08-2026/04-08-2026_[FujiMart][01-08-2026][MB_MT_FUJI][03-08-2026][102001302608000155].pdf
05-08-2026/05-08-2026_[FujiMart][05-08-2026][MB_MT_FUJI][07-08-2026][105001302608000288].pdf
10-08-2026/10-08-2026_[FujiMart][09-08-2026][MB_MT_FUJI][11-08-2026][117003302608000667].pdf
10-08-2026/10-08-2026_[FujiMart][10-08-2026][MB_MT_FUJI][12-08-2026][106003302608000751].pdf
13-07-2026/13-07-2026_[FujiMart][12-07-2026][MB_MT_FUJI][14-07-2026][116001302607000453].pdf
13-07-2026/13-07-2026_[FujiMart][12-07-2026][MB_MT_FUJI][14-07-2026][117003302607000942].pdf
14-07-2026/14-07-2026_[FujiMart][13-07-2026][MB_MT_FUJI][15-07-2026][103001302607000991].pdf
18-07-2026/18-07-2026_[FujiMart][17-07-2026][MB_MT_FUJI][19-07-2026][101003302607001286].pdf
20-07-2026/20-07-2026_[FujiMart][20-07-2026][MB_MT_FUJI][22-07-2026][108003302607001012].pdf
22-07-2026/22-07-2026_[FujiMart][21-07-2026][MB_MT_FUJI][23-07-2026][102001302607001667].pdf
27-07-2026/27-07-2026_[FujiMart][15-07-2026][MB_MT_FUJI][17-07-2026][124003302607000742].pdf
27-07-2026/27-07-2026_[FujiMart][27-07-2026][MB_MT_FUJI][29-07-2026][104001302607001834].pdf
28-07-2026/28-07-2026_[FujiMart][28-07-2026][MB_MT_FUJI][30-07-2026][122003302607001901].pdf
31-07-2026/31-07-2026_[FujiMart][01-08-2026][MB_MT_FUJI][03-08-2026][110003302608000013].pdf
```

**Nhận diện** (`identify_vendor`, `xulydonhang.py:128-129`):
```python
if re.search(r"251000000161", cleaned_text):
    return "FujiMart"
```
Marker số đơn giản, không có `or`. Đã xác nhận có trong cả 15 file mẫu
thật (số thuế nhà cung cấp, xuất hiện cố định trong mọi đơn).

**Xác nhận: FujiMart là "1 trang = 1 đơn"** — cùng nhóm Coop/Lotte/Satra/
Winmart/Emart, khác BigC. Xác nhận qua nhánh `elif vendor == "FujiMart":`
trong `process_file` (`xulydonhang.py:8831-8982`): logic upload
`page_label == '1/1'` giống hệt idiom các vendor khác đã port, không có
trạng thái tích lũy xuyên trang.

**Vị trí trong `identify_vendor`'s thứ tự kiểm tra**: Coop → BigC → Lotte
→ Satra → Emart → Kingfood → CN-HCM → Winmart → SHOPEE-CHOICE →
**FujiMart** → Tiktok → ... Vì Kingfood/CN-HCM/SHOPEE-CHOICE (3 vendor
đứng giữa Winmart và FujiMart trong Python) đều **chưa port**, case
FujiMart trong Go's `vendor.Identify` chỉ cần **nối sau Winmart** (case
cuối cùng hiện có) — không cần chèn giữa.

## Không cần OCR — xác nhận trực tiếp qua cả 2 pipeline

Python dùng `pytesseract` (render trang → ảnh 500dpi → OCR) để lấy tên
chi nhánh, NHƯNG kiểm tra trực tiếp trên 6 file PDF thật khác nhau (cả
qua PyMuPDF lẫn qua Go's `extractPageTexts`) xác nhận: **tên chi nhánh
đã có sẵn trong lớp text thường**, luôn ở vị trí cố định — dòng đầu tiên
có nội dung khác rỗng sau khi trang bắt đầu (dạng `"FujiMart <tên chi
nhánh>"`, ví dụ `"FujiMart T©y S¬n"`, `"FujiMart Hoµng CÇu"`, `"FujiMart
10 TrÇn Phó-Hµ §«ng"`).

**Quyết định: không port OCR.** Trích tên chi nhánh bằng cách tìm dòng
đầu tiên bắt đầu bằng `"FujiMart "` (không dùng chỉ số dòng cố định —
tìm theo prefix chịu được lệch dòng nhỏ giữa các PDF). Giữ Go thuần,
không thêm dependency Tesseract/cgo.

**Giá trị này đi đâu**: Python gán biến `diachigiao`/`tenstore` này thẳng
vào tham số `diachigiao` của `write_to_dondathang_fujimart`, ghi vào cột
**E** (ShipTo) — không phải chỉ để log. Biến `socuahang =
tenstore.split()[0]` (dòng 8917) chỉ dùng trong 1 dòng log/print
(`f'SỐ cửa hàng: FJ{socuahang}'`), **không** ảnh hưởng `makhachhang` hay
bất kỳ tra cứu nào — Go không cần port giá trị `socuahang` này.

## Trích xuất PO/ngày — xác nhận vị trí tương đối bằng cả 2 pipeline

Kiểm tra trực tiếp trên `103001302608001342.pdf` (cả PyMuPDF lẫn Go's
`extractPageTexts`), trình tự các dòng quanh khu vực PO/ngày **giống
hệt nhau giữa 2 pipeline**:
```
N¬i nhËn:
11031
18/08/2026          <- entry_date (giá trị)
103001302608001342  <- po_number (giá trị)
14:43
Sè §¬n:              <- label "Số Đơn:" — offset tham chiếu
Ngµy ®Æt:
```

**entry_date** (`xulydonhang.py:8853-8857`): tìm dòng chứa marker
`"Sè §¬n:"`, lấy dòng **3 vị trí TRƯỚC** dòng marker đó. Đây là kỹ thuật
dựa trên offset vị trí tuyệt đối trong khối "giá trị-trước-label" cố
định của template PDF này — không phải "dòng ngay sau nhãn". Xác nhận
thứ tự tương đối này **giống hệt** trong Go's extraction (chỉ khác 1 dòng
trống ở đầu văn bản Go không có trong Python, không ảnh hưởng offset
tương đối `-3` tính từ marker).

**po_number** (`xulydonhang.py:8885-8887`): tìm dòng có nội dung TRÙNG
KHỚP CHÍNH XÁC với giá trị `entry_date` vừa trích được, lấy dòng **ngay
sau** dòng đó. Xác nhận: trong cả 2 pipeline, dòng chứa `"18/08/2026"`
(entry_date) luôn được theo ngay bởi dòng chứa PO number.

**cancel_date** (`xulydonhang.py:8859`): Python tìm dòng chứa marker
`"Ngµy giao:"`, cắt trên literal marker đó (`split`), lấy phần còn lại
— giả định label và giá trị **CÙNG MỘT DÒNG** (`"Ngµy giao: 20/08/2026"`
trong PyMuPDF's output). **Xác nhận: Go's extraction TÁCH thành 2 dòng
riêng** (`"Ngµy giao:"` rồi `"20/08/2026"` ở dòng kế tiếp) — khác Python,
cùng lớp lệch layout đã gặp ở Emart. **Quyết định thiết kế**: dùng hàm
quét dòng chịu được CẢ 2 trường hợp (cùng dòng HOẶC dòng kế tiếp) — mô
phỏng theo đúng `valueAfterLabel` đã viết cho Emart's `ParseOrderInfo`
(Task 2b), không port nguyên văn cách cắt-cùng-dòng của Python.

**Cross-validate/fallback ngày** (`xulydonhang.py:8862-8884`): nếu
`cancel_date` không khớp `\d{2}/\d{2}/\d{4}$`, gán `"Không tìm thấy"`
rồi tính bù `entry_date + 2 ngày`; logic đối xứng cho `entry_date`. Đây
là logic dự phòng đặc thù, không dùng hàm chung nào — port thẳng, không
đơn giản hoá.

**Địa chỉ giao hàng / khối sản phẩm** (`xulydonhang.py:8895-8907`): cắt
từ `text.rfind("§Þa chØ:")` (marker "Địa chỉ:" — **khác** biến thể mojibake
`"§i¹ chØ:"` xuất hiện sớm hơn trong văn bản, thuộc địa chỉ NCC/vendor,
không phải địa chỉ giao hàng) đến `text.find("VAT")`. Xác nhận trực tiếp:
marker `"§Þa chØ:"` chỉ xuất hiện ĐÚNG 1 LẦN trong văn bản thật (ngay
trước bảng sản phẩm), nên `rfind` không thực sự cần "cuối cùng" — chỉ có
1 lần khớp. Khối cắt ra chứa đúng toàn bộ bảng 5 sản phẩm trong file mẫu.

## Trích xuất sản phẩm — `tach_san_pham_Fujimart` (`xulydonhang.py:7017-7053`)

```python
pattern = re.compile(r"""
    (\d+)\n                     # STT (1, 2, 3...)
    ([\d.]+)\n                  # Quantity 
    ([\d,]+)\n                  # Total Amount
    ([A-Z]+)\n                  # Unit (TUI, HOP...)
    ([\d,]+)\n                  # Unit Price
    (.+?)\n                     # Product Name
    (\d+|[A-Z0-9]+)             # Barcode hoặc Product Code
""", re.VERBOSE | re.MULTILINE)
```
Áp dụng qua `finditer` trên TOÀN BỘ khối văn bản đã cắt (không phải quét
từng dòng như hầu hết vendor khác). Trả về `{Barcode, "Product Name",
Unit, "Unit Price", "Total Price", "OU Qty"}` — nhưng `write_to_dondathang_fujimart`
**chỉ dùng** `Barcode`, `"Total Price"` (đổi tên `dongia`), và `"OU Qty"`.
`"Product Name"` bị bỏ qua hoàn toàn (Excel ghi tên sản phẩm qua tra cứu
`timten_sanpham(barcode)`, không dùng tên trích từ PDF) — Go's `Product`
struct chỉ cần giữ `Barcode`, `OUQty`, `TotalPrice`.

**Bẫy đã xác nhận qua dữ liệu thật**: sau mã vạch (barcode 13 số) của mỗi
sản phẩm, bảng còn in thêm 1 mã nội bộ khác trên dòng riêng (ví dụ
`2006324377`) — regex's `finditer` tự động "trượt" qua dòng thừa này
(vì dòng số nội bộ đó không thỏa `[\d.]+` — nhóm "Quantity" của sản phẩm
kế tiếp — nên `finditer` lùi lại tìm đúng STT kế tiếp). Xác nhận hành vi
này y hệt qua ví dụ thật (`103001302608001342.pdf`, 5 sản phẩm, mỗi dòng
mã nội bộ đều bị bỏ qua đúng cách). Go's `regexp` (RE2) không hỗ trợ
backtracking như Python, nhưng `FindAllStringSubmatch` (non-overlapping,
tìm lại từ vị trí cuối match trước) tạo hiệu ứng tương đương cho pattern
này (không dùng backreference/lookaround) — **cần viết test đối chiếu
trực tiếp với dữ liệu thật để xác nhận, không giả định suông** (giống
cách Emart's `productLinePattern` đã được xác minh).

## Khối trong `write_to_dondathang_fujimart` (`xulydonhang.py:2732-3066`)

| Khối | Dòng | Tóm tắt | Khả năng tái dùng |
|---|---|---|---|
| Dòng header | 2777-2796 | Ghi A/AV/B/C/D/G/L/V/AE/AJ/AM/U/Z/S/T/X/Y/E; S=diengiai (KHÁC Emart — FujiMart's S header dùng luôn `diengiai`, không có biến thể riêng) | `excelwriter.Row` + `fujimartRegionInfo` |
| **Vùng miền** | 2753-2760 | `makhachhang[:2]=="MB"` → HN; else → `LA_KHO2026`/`MT_MN`/`LA`. `makhachhang` LUÔN là hằng số cứng `'MB_MT_FUJI'` (dòng 8919, xác nhận thêm bởi cả 14 file kho lưu trữ đều tag `MB_MT_FUJI`) → nhánh else **không đạt tới trên thực tế** | Theo đúng pattern Winmart/Emart: viết `fujimartRegionInfo` đầy đủ 2 nhánh dù 1 nhánh hiện không đạt tới, để nhất quán kiến trúc |
| Vòng lặp sản phẩm chính | 2798-2839 | SKU/qty/giá chuẩn; **CÓ ghi AU** (case count, giống Winmart/Coop/Satra/Lotte, KHÁC Emart/BigC không ghi AU); **không có** logic bỏ hàng giá-0 (khác Winmart) — mọi sản phẩm trích được đều có dòng riêng | Pattern tô đỏ/comment tái dùng |
| **Khối khuyến mãi tặng kèm theo sản phẩm** | 2949-3007 | Y hệt shape Coop/Satra's `buildPromoBonusRow` (1 lần thử CTKM, không multi-CTKM split trên `"|"` — biến `khuyenmai` không bị `.split('|')`, khác Coop/Emart). **Fallback không-ngoặc: `'KM Bó Kèm - Không Che Barcode'`** (dòng 2973) — chuỗi thứ 4, khác Coop (`'KM Bó Kèm - Che Barcode'`), Winmart (`'KM Giao Rời...'`), Emart (`'KM Rời...'`). **QUAN TRỌNG: fallback này VẪN ghi AP** (dòng 2974-2975, cả `current_row` và `current_row+1`) — khác Winmart/Emart's fallback (không ghi AP) — gần giống Coop's shape hơn (fallback vẫn coi là "gói" và ghi AP) | Tái dùng `buildPromoBonusRow`, override CHỈ text AO tại call site (giữ nguyên hành vi ghi AP mặc định của helper — không cần override AP như Winmart/Emart đã làm) |
| **Khối khuyến mãi cấp hóa đơn** | 3010-3047 | Y hệt shape `buildInvoiceBonusRow` — `Q=kiemtra[0]` (chỉ SKU đầu, không nối chuỗi — giống Winmart/Emart). Fallback: `'KM Bó Kèm - Che Barcode'` (dòng 3044, khớp mặc định chung của `buildInvoiceBonusRow`) — **không cần override** | Không tái dùng `buildInvoiceBonusRow` thẳng (vẫn cần override Q=SKU-đầu-only như Winmart/Emart), nhưng fallback text khớp mặc định nên không cần override thêm |
| Ghi tổng khối lượng vào ô L header | 3049 | `sheet[f"L{start_row}"] = ...` | Khớp `WriteOrderRows`'s `headerDescription` |
| Lưu + log | 3050-3066 | Không port | — |

**`kiemtra[0]` không guard rỗng** (dòng 3025) — cùng crash risk tiềm ẩn
đã gặp ở Winmart/Emart — Go dùng guard sẵn có của `buildInvoiceBonusRow`.

## Phạm vi

### Làm thật
- Trích tên chi nhánh (tìm dòng bắt đầu `"FujiMart "`), entry_date (offset
  -3 từ marker `"Sè §¬n:"`), po_number (dòng ngay sau dòng entry_date),
  cancel_date (quét dòng chịu được cùng-dòng-hoặc-dòng-kế của
  `"Ngµy giao:"`), cross-validate/fallback ±2 ngày.
- Cắt khối sản phẩm (`rfind("§Þa chØ:")` → `find("VAT")`) + trích sản
  phẩm (`ExtractProducts`, mirror `tach_san_pham_Fujimart`).
- `fujimartRegionInfo`: 2 nhánh đầy đủ theo thiết kế đã chốt.
- Khối khuyến mãi tặng kèm: tái dùng `buildPromoBonusRow`, override CHỈ
  text AO fallback (`"KM Bó Kèm - Không Che Barcode"`), giữ nguyên hành
  vi ghi AP mặc định.
- Khối khuyến mãi cấp hóa đơn: port riêng (như Winmart/Emart), dùng guard
  rỗng có sẵn.
- Chèn `vendor.Identify`'s case FujiMart — nối sau Winmart (case cuối).
- Dispatch per-page như các vendor "1 trang = 1 đơn" khác.
- File mới `GO/internal/processing/fujimart/` + `fujimart_processor.go` +
  test tương ứng.
- **Copy PDF thật vào testdata ổn định NGAY từ Task tương ứng** (không
  đợi sự cố như Emart) — cần hỏi lại chủ dự án trước khi commit.

### Không làm
- OCR (đã chốt không cần — xem mục riêng ở trên).
- Kingfood, CN-HCM, SHOPEE-CHOICE — chưa port, không nằm giữa Winmart và
  FujiMart trong Go hiện tại nên không ảnh hưởng vị trí chèn.
- Upload PDF lên Google Drive.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go              # + case FujiMart, NỐI SAU Winmart (case cuối)
  fujimart/
    extract.go                # ParseOrderInfo (tên chi nhánh/PO/ngày,
                              # dùng line-scan chịu lệch layout Go-vs-
                              # Python), ExtractProducts
  fujimart_processor.go        # processFujimartSegment
  fujimart_processor_test.go
  fujimart_golden_test.go

GO/internal/processing/fujimart/testdata/
  realpdfs/*.pdf               # PDF thật ổn định, git-tracked (cần chốt
                              # với chủ dự án trước khi commit)
  fixtures/*.json
  generate_fixtures.py
```

## Dữ liệu ngoài & phụ thuộc mạng

- **`settings.ini`**: đã xác nhận có sẵn gid cho `FUJIMART` (dòng 10) —
  không cần sửa.
- Không cần fuzzy-match customer-code (mã KH luôn hardcode `MB_MT_FUJI`).

## Testing

- Unit test cho `ParseOrderInfo`/`ExtractProducts` trong `fujimart/`,
  dùng dữ liệu thật đã xác minh qua cả 2 pipeline ở trên.
- Test riêng cho fallback AO/AP của khối khuyến mãi tặng kèm (xác nhận
  fallback VẪN ghi AP, khác Winmart/Emart).
- Test riêng cho `fujimartRegionInfo` (cả 2 nhánh, dù 1 nhánh không đạt
  tới với input thật hiện tại).
- Golden fixture: đối chiếu với các PDF thật đã xác định vị trí ổn định.

## Rủi ro / lưu ý

- **Offset vị trí tuyệt đối (`-3` dòng từ marker) cho entry_date** là kỹ
  thuật dễ vỡ nếu layout PDF thay đổi dù chỉ chút ít — đã xác nhận đúng
  trên mẫu hiện có qua cả 2 pipeline, nhưng cần xác nhận lại trên TOÀN
  BỘ 15 file khi chạy golden fixture, không chỉ 1 file đã kiểm tra lúc
  brainstorm.
- **Chưa xác nhận `finditer`-trượt-dòng-thừa của `tach_san_pham_Fujimart`
  có tái tạo đúng qua Go's non-backtracking `FindAllStringSubmatch`** —
  cần test đối chiếu trực tiếp, không giả định suông (dù RE2 không có
  backreference/lookahead trong pattern này nên rủi ro thấp).
- **Quyết định commit PDF thật vào git** cần hỏi lại chủ dự án riêng cho
  FujiMart, không tự động áp dụng quyết định đã có cho Emart.
