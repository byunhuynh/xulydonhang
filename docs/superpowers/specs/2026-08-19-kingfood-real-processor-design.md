# Thiết kế: RealProcessor cho Kingfood (Phase 2b-7 của dự án refactor — vendor cuối cùng trong roadmap)

## Bối cảnh

Phase 2b-1 đến 2b-6 (Lotte, Satra, BigC, Winmart, Emart, FujiMart) đã hoàn
tất và đóng. Phase 2b-7 làm **Kingfood** — vendor cuối cùng trong roadmap
7-vendor ban đầu. **9 file PDF Kingfood thật khả dụng ngay**, toàn bộ nằm
trong kho lưu trữ `đơn hàng/mẫu đơn hàng/*/` (thư mục sống
`đơn hàng/08-2026/` hiện chỉ còn 10 file không liên quan Kingfood — xác
nhận lại tại thời điểm viết spec này, đúng theo cảnh báo đã ghi trong
project memory).

### Kế thừa từ các phase trước

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ".
Cùng hạ tầng dùng chung: `excelwriter.Row`/`WriteOrderRows`,
`pricing.Index`/`FetchIndex`, `processor_shared.go`'s
`regionInfo`/`closeEnough`/`buildPromoBonusRow`/`buildInvoiceBonusRow`/
`coopDebtDays`/`stripBlankLines`/`xPlus1Pattern`.

### Bài học từ FujiMart's close — áp dụng ngay từ đầu

1. **Task 0 smoke test đã chạy TRƯỚC KHI viết spec này, qua NHIỀU file
   (3), không chỉ 1** — đúng khuyến nghị đã ghi trong project memory sau
   khi FujiMart's Task 0 (chỉ chạy 1 file) bỏ sót một lỗi chỉ xuất hiện ở
   3/15 file khác. Kết quả: phát hiện một lớp lệch layout Go-vs-Python
   **hoàn toàn mới**, chưa từng gặp ở 6 vendor trước — xem mục riêng bên
   dưới.
2. **Đã xác nhận lại trực tiếp thứ tự vendor thật** trong
   `identify_vendor` (`xulydonhang.py:90-129`), không tin bất kỳ ghi chú
   nào từ vendor trước (kể cả bản thân spec/plan FujiMart từng có 1 lỗi
   dạng này). Thứ tự thật: `...Emart(111) → Kingfood(114) →
   [CN-HCM, chưa port](118) → Winmart(121) → [SHOPEE-CHOICE, chưa port]
   (125) → FujiMart(128)...`. **Kingfood là vendor ĐẦU TIÊN trong dự án
   này không thể chỉ nối cuối chuỗi `vendor.Identify` hiện có** — xem mục
   "Vị trí chèn" bên dưới.
3. **⚠️ Rủi ro kiến trúc đang hoạt động, không riêng Kingfood**: thư mục
   `đơn hàng/08-2026/` không phải nguồn dữ liệu ổn định (ứng dụng thật
   đang chạy song song). Plan này sẽ áp dụng ngay từ Task tương ứng thiết
   kế copy PDF thật tìm được vào testdata ổn định, git-tracked
   (`GO/internal/processing/kingfood/testdata/realpdfs/`) — **cần hỏi lại
   chủ dự án riêng cho Kingfood trước khi commit, không tự động áp dụng
   quyết định đã có cho Emart/FujiMart**.

## Dữ liệu đối chiếu (golden corpus)

**9 file PDF Kingfood thật khả dụng ngay** (tại thời điểm viết spec — ảnh
chụp tức thời), toàn bộ trong kho lưu trữ, đã đổi tên theo định dạng
`<ngày xử lý>_[Kingfood][entryDate][MN_MT_KFMSL][cancelDate][PO gốc].pdf`:

```
03-08-2026/03-08-2026_[Kingfood][03-08-2026][MN_MT_KFMSL][05-08-2026][PO1002601888].pdf
06-08-2026/06-08-2026_[Kingfood][06-08-2026][MN_MT_KFMSL][08-08-2026][PO1002605686].pdf
10-07-2026/10-07-2026_[Kingfood][10-07-2026][MN_MT_KFMSL][13-07-2026][PO1002575355].pdf
13-07-2026/13-07-2026_[Kingfood][13-07-2026][MN_MT_KFMSL][15-07-2026][PO1002578376].pdf
13-08-2026/13-08-2026_[Kingfood][10-08-2026][MN_MT_KFMSL][14-08-2026][PO1002610000].pdf
16-07-2026/16-07-2026_[Kingfood][16-07-2026][MN_MT_KFMSL][18-07-2026][PO1002582369].pdf
20-07-2026/20-07-2026_[Kingfood][20-07-2026][MN_MT_KFMSL][22-07-2026][PO1002586301].pdf
28-07-2026/28-07-2026_[Kingfood][27-07-2026][MN_MT_KFMSL][29-07-2026][PO1002594163].pdf
30-07-2026/30-07-2026_[Kingfood][30-07-2026][MN_MT_KFMSL][01-08-2026][PO1002597903].pdf
```

Xác nhận qua kiểm tra trực tiếp: file `20-07-2026_...[PO1002586301].pdf`
có **2 sản phẩm** (2 mã vạch khác nhau trong cùng 1 đơn) — corpus không
chỉ toàn đơn 1-sản-phẩm, cần đảm bảo `ExtractProducts` xử lý đúng vòng
lặp nhiều dòng sản phẩm.

**Nhận diện** (`identify_vendor`, `xulydonhang.py:114-115`):
```python
if re.search(r"0313403198", cleaned_text):
    return "Kingfood"
```
Marker số đơn giản (mã số thuế Kingfood), không có `or`. Xác nhận có
trong toàn bộ 9 file mẫu thật (`MST:\n0313403198`, xem trích văn bản thật
ở mục dưới).

**Xác nhận: Kingfood là "1 trang = 1 đơn"** — cùng nhóm Coop/Lotte/Satra/
Winmart/Emart/FujiMart, khác BigC. Xác nhận qua nhánh
`elif vendor == "Kingfood":` trong `process_file`
(`xulydonhang.py:9230-9310`): logic upload theo `page_label == '1/1'`
giống hệt idiom các vendor khác đã port, không có trạng thái tích lũy
xuyên trang. Cả 9 file mẫu đều 1 trang duy nhất.

### Vị trí chèn trong Go's `vendor.Identify`

Chuỗi hiện tại trong Go: `Coop → BigC → Lotte → Satra → Emart → Winmart →
FujiMart` — khớp đúng thứ tự tương đối thật của Python trong số các
vendor **đã port**. Nhưng Kingfood (thứ tự thật: ngay sau Emart, trước
Winmart) phải **CHÈN GIỮA Emart và Winmart**, không phải nối cuối sau
FujiMart — đây là lần đầu tiên trong dự án việc chèn case mới không đơn
giản là "thêm vào cuối". CN-HCM (đứng giữa Kingfood và Winmart trong
Python) chưa port nên không ảnh hưởng vị trí chèn tương đối giữa Kingfood
và Winmart trong Go.

## ⚠️ Lớp lệch layout Go-vs-Python MỚI: tab thay vì space trong nhãn nhiều từ

Chạy trực tiếp `extractPageTexts` (Go, pipeline thật) trên 3 file PDF
Kingfood thật và so sánh với PyMuPDF's `get_text("text")` trên CÙNG file:

**Go's `extractPageTexts`:**
```
PO	Number:
PO1002601888
Nơi	giao:
KHO	SEEDLOG
Ngày	Giao	Hàng	Dự	Kiến:
05-08-2026
Ngày	Giao	Hàng	NCC	Xác
Nhận:
05-08-2026
Ngày	Đặt	Hàng:
03-08-2026
```
(`\t` = ký tự tab thật, không phải hiển thị canh lề)

**PyMuPDF's `get_text("text")`, CÙNG FILE:**
```
Page 1 / 2
PO Number:
PO1002601888
Nơi giao:
KHO SEEDLOG
Ngày Giao Hàng Dự Kiến:
05-08-2026
Ngày Giao Hàng NCC Xác
Nhận:
05-08-2026
Ngày Đặt Hàng:
03-08-2026
```

**Phát hiện**: vị trí xuống dòng (`\n`) HOÀN TOÀN GIỐNG NHAU giữa 2
pipeline (kể cả việc nhãn "Ngày Giao Hàng NCC Xác Nhận:" bị tách thành 2
dòng vật lý trong CẢ HAI, một lệch layout đã tồn tại sẵn trong chính
Python và được Python's `\s*` (khớp cả `\n`) xử lý đúng) — nhưng **ký tự
phân cách GIỮA các từ trong cùng một dòng nhãn khác nhau**: Go chèn tab
(`\t`), PyMuPDF chèn space thường. Đây là lớp lệch MỚI, khác hẳn 6 lớp đã
gặp trước đó (tất cả đều là khác biệt về VỊ TRÍ xuống dòng, chưa từng là
khác biệt về KÝ TỰ PHÂN CÁCH nội dòng).

**Ảnh hưởng thực tế**: Python's regex cho `po_number`/`entry_date` dùng
literal space giữa các từ trong nhãn (`r"PO Number:\s*\n..."`,
`r"Ngày Đặt Hàng:\s*\n..."`) — khớp đúng với PyMuPDF's text (space thật)
nhưng sẽ KHÔNG khớp nếu áp trực tiếp lên Go's text (tab thay vì space).
Riêng `cancel_date`'s regex dùng `\s*` giữa MỌI từ
(`r"Ngày\s*Giao\s*Hàng\s*NCC\s*Xác\s*Nhận:..."`) nên tình cờ khớp được cả
2 loại ký tự phân cách — không phải regex này "thiết kế đúng cho tab", mà
`\s` trong Python's `re` module vốn khớp cả tab/space/newline.

**Quyết định thiết kế**: chuẩn hoá TOÀN BỘ text bằng
`strings.ReplaceAll(text, "\t", " ")` **ngay đầu vào**, trước khi tách
dòng hay khớp bất kỳ marker nào — khôi phục lại đúng hình dạng
space-separated giống PyMuPDF, không cần viết 2 phiên bản logic khớp
(tab-tolerant và space-tolerant) song song. Đơn giản hơn nhiều so với kỹ
thuật "chấp nhận cả 2 layout dòng" đã dùng cho Emart/FujiMart, vì đây chỉ
là 1 phép thay thế ký tự toàn cục, không phải logic quét dòng có điều
kiện.

## Trích xuất PO/ngày (`xulydonhang.py:9230-9263`, sau khi chuẩn hoá tab→space)

Cấu trúc dòng thật (đã xác nhận giống hệt nhau giữa PyMuPDF và Go, sau
chuẩn hoá):
```
PO Number:
PO1002601888        <- po_number (dòng NGAY SAU nhãn)
Nơi giao:
KHO SEEDLOG
Ngày Giao Hàng Dự Kiến:
05-08-2026
Ngày Giao Hàng NCC Xác
Nhận:
05-08-2026           <- cancel_date (dòng NGAY SAU nhãn, nhãn tự nó chiếm 2 dòng)
Ngày Đặt Hàng:
03-08-2026           <- entry_date (dòng NGAY SAU nhãn)
```

**po_number** (`xulydonhang.py:9239-9240`):
```python
po_number = re.search(r"PO Number:\s*\n([^\n]*\n)?([^\n]*)", tranggoc)
po_number = po_number.group(1).strip() if po_number else "Không tìm thấy PO Number"
```
Dù regex có 2 group tùy chọn (`group(1)` và `group(2)`), code chỉ dùng
`.group(1)` — xác nhận qua kiểm tra thật: vì `[^\n]*\n` (group 1, có `?`
nhưng luôn khớp được kể cả chuỗi rỗng, miễn có `\n` phía sau) luôn ưu tiên
khớp NGAY DÒNG kế tiếp sau nhãn, `.group(1)` **trong thực tế luôn chính
là "dòng ngay sau nhãn"**, không phải một trường hợp biên hiếm gặp. Thiết
kế Go: hàm quét dòng đơn giản — tìm dòng khớp CHÍNH XÁC nhãn (sau chuẩn
hoá khoảng trắng), trả về dòng kế tiếp.

**entry_date** (`xulydonhang.py:9243-9247`): cùng shape với `po_number`,
nhãn `"Ngày Đặt Hàng:"`, sau đó `datetime.strptime(entry_date, "%d/%m/%Y")`
— parse thẳng, KHÔNG có logic cross-validate/fallback ±N ngày nào (khác
hẳn Winmart/Emart/FujiMart) — nếu dòng lấy được không đúng định dạng
`dd-mm-yyyy`, Python's `strptime` sẽ **crash thật** (không try/except bao
quanh). Go's port: nếu không parse được, trả `ok=false` (thất bại sạch),
không port hành vi crash.

**cancel_date** (`xulydonhang.py:9249-9262`): nhãn nhiều dòng
`"Ngày Giao Hàng NCC Xác Nhận:"` (tự nó tách "...Xác" / "Nhận:" thành 2
dòng vật lý trong CẢ 2 pipeline, không phải lỗi Go) — cần hàm tìm nhãn
chấp nhận nhãn trải qua nhiều dòng liên tiếp (nối các dòng bằng space rồi
so khớp cụm từ đầy đủ), trả về dòng NGAY SAU dòng cuối cùng của nhãn.
Cùng `datetime.strptime` không try/except — Go trả `ok=false` nếu
không parse được.

**Định dạng ngày trong PDF là `dd-mm-yyyy` (gạch ngang)**, Python
chuyển sang `dd/mm/yyyy` (gạch chéo) trước khi ghi cột A/D — Go port cần
parse `dd-mm-yyyy` rồi format lại thành `dd/mm/yyyy`.

## Địa chỉ giao hàng & mã khách hàng: cả 2 đều hardcode cứng, không trích từ PDF

```python
store_code = "MN_MT_KFMSL"
delivery = 'Số 324, đường ĐT743A, Phường Đông Hoà, Thành phố Hồ Chí Minh'
```
(`xulydonhang.py:9273-9274`) — xác nhận: dòng
`"Địa chỉ giao\nhàng:\nSố 324, đường ĐT743A, ..."` CÓ xuất hiện thật
trong văn bản PDF (trùng khớp y hệt giá trị hardcode), nhưng Python
**không đọc từ PDF** — dùng thẳng hằng số. Go port giữ nguyên: 2 hằng số,
không cần logic trích xuất địa chỉ nào. Đơn giản hơn cả FujiMart (vẫn cần
trích 1 nửa từ text) lẫn Winmart/Satra (cần fuzzy-match).

## Trích xuất bảng sản phẩm

### Cắt vùng bảng — `lamsachdonhang_kingfood` (`xulydonhang.py:6672-6695`)

```python
start_pattern = re.compile(r"Khu vực", re.I)
end_marker = "TỔNG CỘNG"
```
Tìm vị trí cuối cụm `"Khu vực"` (không phân biệt hoa/thường) → lấy phần
sau đó → tìm `"TỔNG CỘNG"` (dòng riêng, `(?<=\n)`) → cắt đến đó. Xác nhận
qua dữ liệu thật: `"Khu vực"` xuất hiện ĐÚNG 1 LẦN — là tiêu đề cột cuối
cùng của bảng, ngay trước dòng dữ liệu sản phẩm đầu tiên (STT=1). Sau
chuẩn hoá tab→space, marker này không bị ảnh hưởng gì (chỉ 2 từ, không
nằm giữa tab). Nếu không tìm thấy, trả về chuỗi cố định
`"Không có sản phẩm"` (không crash) — Go port: trả `nil`/`ok=false` sạch.

### Trích từng sản phẩm — `laydanhsachsanpham_kingfood` (`:6698-6758`)

```python
pattern = re.compile(r"""
    (?P<stt>\d+)\s*\n
    (?P<barcode>\d{13})\s*\n
    (?P<name>(?:.+\n)+?)
    (?P<unit>HỘP|TÚI|CHAI|LON|GÓI)\s*\n
    (?P<quantity>[\d.]+)\s*\n
    \d+\s*\n
    .+\s*\n
    (?:.*\n){4}
    (?P<price>[0-9.,]+)
""", re.VERBOSE)
```
Áp qua `finditer` trên TOÀN VĂN BẢN đã cắt (không quét từng dòng thủ
công). **Đối chiếu trực tiếp với dòng dữ liệu thật của 1 sản phẩm** (đã
chuẩn hoá tab→space):
```
1                                          <- stt
8936156732620                              <- barcode (13 số)
BLUE - VIÊN GIẶT XẢ PHẤN HỒNG TÚI          <- name (dòng 1)
30 VIÊN                                    <- name (dòng 2, cụm "quy cách" bị regex nuốt vào name vì chưa khớp unit)
TÚI                                        <- unit (khớp nhóm HỘP|TÚI|CHAI|LON|GÓI, đứng riêng 1 dòng)
300                                        <- quantity
12                                         <- \d+ (Số lượng Thùng/Pack — bỏ qua, không capture tên riêng)
25 Thùng                                   <- .+ (Thực nhận theo SL đặt — bỏ qua)
102.143                                    <- (?:.*\n){4} dòng 1/4: Đơn giá chào (-VAT)
27%                                        <- dòng 2/4: CK trực tiếp
0%                                         <- dòng 3/4: CK khai trương/ĐH đầu tiên
30%                                        <- dòng 4/4: % KM Giảm Giá
52.195,073                                 <- price (Đơn giá cuối, SAU khi trừ mọi chiết khấu)
```
**`name` group tham lam-tối-thiểu (non-greedy) nuốt CẢ dòng "30 VIÊN"**
vào tên sản phẩm (vì dòng đó chưa khớp `unit`), rồi bị
`re.sub(r'\s+', ' ', ...)` chuẩn hoá khoảng trắng — nhưng
`write_to_dondathang_kingfood` **không dùng** `"Product Name"` từ kết quả
này (ghi cột S qua `timten_sanpham(barcode)`, tra cứu riêng) — Go's
`Product` struct **không cần giữ tên sản phẩm trích từ PDF**, chỉ cần
`Barcode`, `OUQty` (=quantity), `TotalPrice` (=price, **tên field gây
nhầm — thực chất là ĐƠN GIÁ CUỐI SAU CHIẾT KHẤU trên 1 đơn vị, KHÔNG phải
tổng tiền dòng** — xem mục "Đọc giá" bên dưới).

**⚠️ Định dạng số Việt Nam — KHÁC MỌI VENDOR TRƯỚC ĐÓ**: `quantity` chỉ
strip dấu chấm (`.replace('.', '')` — dấu chấm ở đây LÀ dấu phân cách
nghìn, vd `"1.464"` → `1464`). Nhưng `price` xử lý PHỨC TẠP HƠN:
```python
price = float(price_str.replace('.', '').replace(',', '.'))
```
Tức `"52.195,073"` → bỏ dấu chấm (`"52195,073"`) → đổi dấu phẩy thành dấu
chấm thập phân (`"52195.073"`) → `52195.073`. Đây là định dạng số kiểu
châu Âu/Việt Nam (chấm=nghìn, phẩy=thập phân) — **hoàn toàn khác** cách
các vendor trước (Winmart/FujiMart/Emart chỉ strip dấu phẩy đơn thuần,
không có dấu thập phân dạng này trong dữ liệu của họ). Go port: hàm
`parseNumericField` dùng chung (`bigc_processor.go`) **không xử lý được
định dạng này** — cần hàm riêng cho Kingfood
(`parseKingfoodPrice` hoặc tương tự), KHÔNG tái dùng `parseNumericField`
cho trường `price` (vẫn dùng được cho `quantity`, vì `quantity` chỉ có
dấu chấm-nghìn, không có phần thập phân dấu phẩy).

**Bẫy "trượt dòng thừa"**: không có mã nội bộ thừa như FujiMart — cấu
trúc chặt bởi `(?:.*\n){4}` (đúng 4 dòng cố định trước `price`), rủi ro
lệch số dòng thấp hơn FujiMart, nhưng **vẫn cần** test đối chiếu trực
tiếp với text thật (không giả định `(?:.*\n){4}` luôn đúng 4 dòng cho mọi
sản phẩm — ví dụ nếu 1 sản phẩm nào đó dùng "Không giới hạn" hay giá trị
đa dòng cho 1 trong 4 trường bị bỏ qua, số dòng sẽ lệch. Cần đối chiếu cả
9 file, không chỉ mẫu đã kiểm tra ở trên).

## Khối trong `write_to_dondathang_kingfood` (`xulydonhang.py:3848-4196`)

| Khối | Dòng | Tóm tắt | Khả năng tái dùng |
|---|---|---|---|
| Dòng header | 3886-3915 | Ghi A/AV/B/C/D/G/L/V/AE/AJ/AM/U/Z/S/T/X/Y/E. `S{start_row}` = `f"{vendor} {po_number}"`, cùng giá trị với `diengiai` (biến dùng cho `L`/dòng sản phẩm) — đã đối chiếu trực tiếp 2 f-string, thực chất giống hệt nhau, không có khác biệt | `excelwriter.Row` + `kingfoodRegionInfo` |
| **Vùng miền** | 3871-3883 | `makhachhang[:2]=="MB"` → HN; else nếu `makhachhang=="MN_MT_JM0001"` → `LA_TP`/`MT_MN`/`LA`; else → `LA_KHO2026`/`MT_MN`/`LA`. `makhachhang` LUÔN là hằng số cứng `"MN_MT_KFMSL"` (không bắt đầu `"MB"`, không phải `"MN_MT_JM0001"`) → luôn rơi vào nhánh `else`-`else` (`LA_KHO2026`) trên thực tế | Viết đủ 3 nhánh (không phải 2 như FujiMart/Winmart — đây là vendor ĐẦU TIÊN có nhánh giữa `"MN_MT_JM0001"` đặc biệt) để nhất quán kiến trúc dù chỉ 1 nhánh đạt tới |
| Vòng lặp sản phẩm chính | 3922-4071 | SKU/qty/giá chuẩn; **CÓ ghi AU** (case count, giống FujiMart/Winmart/Coop/Satra/Lotte); **không có** logic bỏ hàng giá-0 (khác Winmart); `AR{current_row} = product.get("Mahang", '')` — cột **AR mới, chưa vendor nào trước đây ghi**. Giá trị luôn rỗng trên thực tế vì `Product` dict không có key `"Mahang"` (`laydanhsachsanpham_kingfood` không tạo key này) — **quyết định: bỏ qua cột AR hoàn toàn, không thêm field mới vào `excelwriter.Row`** (khớp mục "Không làm" bên dưới), trừ khi golden fixture cho thấy một fixture thật có giá trị AR khác rỗng (bằng chứng thật mới đủ để đổi quyết định này) | Pattern tô đỏ/comment tái dùng y hệt vendor trước |
| **Khối khuyến mãi tặng kèm theo sản phẩm** | 4074-4128 | Y hệt shape Coop's `buildPromoBonusRow` (1 lần thử CTKM per sản phẩm, không multi-CTKM `"|"`-split). Fallback không-ngoặc: `'KM Giao Rời - Không Che Barcode'` (dòng 4096) — **giống hệt chuỗi fallback của Winmart**, khác Coop/FujiMart/Emart. **Fallback này KHÔNG ghi AP** (chỉ nhánh có `cachbokem` mới ghi AP, dòng 4093-4094) — giống Winmart/Emart, khác FujiMart/Coop | Tái dùng `buildPromoBonusRow`, override text AO fallback + xoá AP như đã làm cho Winmart/Emart (không giữ AP mặc định của helper) |
| **Khối khuyến mãi cấp hóa đơn** | 4131-4177 | Y hệt shape `buildInvoiceBonusRow` — `Q=kiemtra[0]` (chỉ SKU đầu, giống Winmart/Emart/FujiMart). Fallback: `'KM Bó Kèm - Che Barcode'` (dòng 4171, khớp mặc định chung của `buildInvoiceBonusRow`) — không cần override | Không tái dùng `buildInvoiceBonusRow` thẳng (vẫn cần override Q=SKU-đầu-only), fallback text khớp mặc định |
| Ghi tổng khối lượng vào ô L header | 4181 | `sheet[f"L{start_row}"] = ...` | Khớp `WriteOrderRows`'s `headerDescription` |
| Lưu + log | 4182-4196 | Không port | — |

**`kiemtra[0]` không guard rỗng** (dòng 4147) — cùng crash risk tiềm ẩn
đã gặp ở Winmart/Emart/FujiMart — Go dùng guard sẵn có của
`buildInvoiceBonusRow`.

**Đọc giá (`giahoadon`)**: `giahoadon = dongia = float(product["Total Price"])`
(dòng 3925, 3970) — do "Total Price" thực chất là ĐƠN GIÁ CUỐI 1 đơn vị
(xem mục trích xuất sản phẩm ở trên), `giahoadon` ở đây LÀ giá đơn vị,
**không** chia cho `qty_ord_pcs` như FujiMart/Winmart (họ có `dongia` là
tổng tiền dòng, cần chia). Kingfood's `dongia` đã sẵn là giá đơn vị —
port thẳng `invoicePrice := totalPrice` (không chia), khác hẳn
`invoicePrice := totalPrice / ouQty` của FujiMart/Winmart. **Đặt tên
field `TotalPrice` trong Go's `Product` struct dù ngữ nghĩa là giá đơn
vị, để nhất quán tên cột nguồn `"Total Price"` trong dữ liệu — ghi rõ
trong doc comment để người đọc sau không nhầm.**

## Phạm vi

### Làm thật
- Chuẩn hoá `text = strings.ReplaceAll(text, "\t", " ")` **ngay đầu vào**
  trước mọi bước trích xuất.
- Trích `poNumber`/`entryDate`/`cancelDate` qua hàm quét dòng: tìm nhãn
  (đơn dòng hoặc 2-dòng liên tiếp cho `cancelDate`), lấy dòng ngay sau.
  Parse `dd-mm-yyyy` → format `dd/mm/yyyy`. Không cross-validate/fallback
  (khác FujiMart/Winmart/Emart) — `ok=false` sạch nếu parse ngày thất
  bại, không port hành vi crash `strptime` của Python.
- `storeCode = "MN_MT_KFMSL"`, `delivery = "Số 324, đường ĐT743A, Phường
  Đông Hoà, Thành phố Hồ Chí Minh"` — 2 hằng số cứng, không trích PDF.
- Cắt khối sản phẩm (`"Khu vực"` → `"TỔNG CỘNG"`) + trích sản phẩm
  (`ExtractProducts`, mirror `laydanhsachsanpham_kingfood`), dùng hàm
  parse số kiểu Việt Nam riêng cho `price` (chấm=nghìn, phẩy=thập phân).
- `kingfoodRegionInfo`: đủ 3 nhánh (MB / `MN_MT_JM0001` / else).
- Khối khuyến mãi tặng kèm: tái dùng `buildPromoBonusRow`, override text
  AO fallback (`"KM Giao Rời - Không Che Barcode"`) **và xoá AP mặc định**
  (giống Winmart/Emart, khác FujiMart).
- Khối khuyến mãi cấp hóa đơn: port riêng (như Winmart/Emart/FujiMart),
  dùng guard rỗng có sẵn.
- **Chèn `vendor.Identify`'s case Kingfood GIỮA Emart và Winmart** (không
  nối cuối).
- Dispatch per-page như các vendor "1 trang = 1 đơn" khác.
- File mới `GO/internal/processing/kingfood/` + `kingfood_processor.go` +
  test tương ứng.
- **Copy 9 PDF thật vào testdata ổn định** — cần hỏi lại chủ dự án trước
  khi commit (không giả định giống quyết định Emart/FujiMart).

### Không làm
- CN-HCM, SHOPEE-CHOICE — chưa port, đứng giữa Kingfood/Winmart và
  Winmart/FujiMart trong Python nhưng không ảnh hưởng vị trí chèn tương
  đối giữa các vendor ĐÃ port.
- Cross-validate/fallback ngày ±N ngày — Python không có logic này cho
  Kingfood (khác FujiMart/Winmart/Emart), không tự thêm.
- Cột AR (`Mahang`) — giá trị luôn rỗng trên thực tế, không cần
  `excelwriter.Row` field mới trừ khi golden fixture cho thấy khác.
- Upload PDF lên Google Drive.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go              # + case Kingfood, CHÈN GIỮA Emart và Winmart
  kingfood/
    extract.go                # ParseOrderInfo (PO/ngày, quét dòng sau
                              # chuẩn hoá tab→space), ExtractProducts
                              # (regex + parse số kiểu VN cho price)
  kingfood_processor.go        # processKingfoodSegment
  kingfood_processor_test.go
  kingfood_golden_test.go

GO/internal/processing/kingfood/testdata/
  realpdfs/*.pdf               # PDF thật ổn định, git-tracked (cần chốt
                              # với chủ dự án trước khi commit)
  fixtures/*.json
  generate_fixtures.py
```

## Dữ liệu ngoài & phụ thuộc mạng

- **`settings.ini`**: cần xác nhận có gid cho `KINGFOOD` — **đã xác nhận
  có sẵn** (`settings.ini:8`, `KINGFOOD = 281168437`), không cần sửa.
- Không cần fuzzy-match customer-code (mã KH luôn hardcode
  `MN_MT_KFMSL`), không cần OCR (địa chỉ giao hàng cũng hardcode, không
  trích từ PDF).

## Testing

- Unit test cho `ParseOrderInfo`/`ExtractProducts` trong `kingfood/`,
  dùng dữ liệu thật đã xác minh qua cả 2 pipeline ở trên (bao gồm cả
  case tab→space chuẩn hoá và nhãn 2-dòng cho `cancelDate`).
- Test riêng cho hàm parse số kiểu Việt Nam (`"52.195,073"` → `52195.073`,
  `"1.252.682"` → `1252682`) — bao gồm case không có phần thập phân.
- Test riêng cho fallback AO/AP của khối khuyến mãi tặng kèm (xác nhận
  fallback KHÔNG ghi AP, giống Winmart/Emart, khác FujiMart).
- Test riêng cho `kingfoodRegionInfo` (đủ 3 nhánh, dù chỉ 1 nhánh đạt tới
  với input thật hiện tại).
- Test riêng xác nhận `invoicePrice` = giá đơn vị trực tiếp (KHÔNG chia
  cho số lượng) — khác FujiMart/Winmart, dễ nhầm nếu copy-paste code cũ.
- Golden fixture: đối chiếu với 9 PDF thật, bao gồm file có 2 sản phẩm
  (`PO1002586301`) để xác nhận vòng lặp nhiều sản phẩm hoạt động đúng.

## Rủi ro / lưu ý

- **Lớp lệch layout tab-vs-space mới phát hiện qua Kingfood** — đã xử lý
  bằng chuẩn hoá toàn cục, nhưng cần xác nhận lại trên TOÀN BỘ 9 file khi
  chạy golden fixture (không chỉ 3 file đã kiểm tra ở Task 0), phòng
  trường hợp có file dùng font/layout khác gây ra ký tự phân cách khác
  tab/space (ví dụ non-breaking space).
- **`(?:.*\n){4}` (bỏ qua đúng 4 dòng cố định) trước khi lấy `price`** dễ
  vỡ nếu 1 trong 4 trường bị bỏ qua chiếm nhiều hơn 1 dòng ở 1 file nào
  đó — cần đối chiếu cả 9 file, không giả định cấu trúc cố định từ 1 mẫu.
- **Định dạng số kiểu Việt Nam (chấm=nghìn, phẩy=thập phân) chỉ áp dụng
  cho trường `price` của Kingfood** — không được vô tình áp dụng hàm này
  cho các trường khác hoặc tái dùng nhầm cho vendor khác (mọi vendor
  trước chỉ dùng dấu phẩy làm phân cách nghìn kiểu Mỹ).
- **Không có cross-validate/fallback ngày** — nếu 1 trong 3 ngày parse
  thất bại trên 1 file thật nào đó trong golden fixture, thiết kế
  `ok=false` sạch là đúng theo policy dự án, KHÔNG thêm logic fallback
  Python không có.
- **Quyết định commit PDF thật vào git** cần hỏi lại chủ dự án riêng cho
  Kingfood, không tự động áp dụng quyết định đã có cho Emart/FujiMart.
