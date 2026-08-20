# Thiết kế: RealProcessor cho JMart (vendor mới, ngoài roadmap Phase 2b ban đầu)

## Bối cảnh

Phase 2b (7 vendor: Lotte, Satra, BigC, Winmart, Emart, FujiMart, Kingfood)
đã hoàn tất, cùng với Coop (Phase 2a). Người dùng yêu cầu tiếp tục "hoàn
thành chuyển đổi app" — khảo sát cho thấy đây là roadmap 5 giai đoạn lớn
hơn nhiều (xem `docs/superpowers/specs/2026-08-13-go-wails-skeleton-design.md`),
trong đó việc port thêm vendor thuộc Phase 3 (mở rộng `RealProcessor` ra
toàn bộ vendor trong `settings.ini`). Người dùng đã chọn: chỉ tiếp tục
port vendor (không làm Zalo/MISA automation/production cutover trong giai
đoạn này), và trong số 8 vendor còn thiếu dữ liệu thật
(BHX/Farmer/CN-HCM/MR.DIY/JIT/JV-Mart/JMart/BC Mart), **chỉ JMart có 1 file
PDF thật khả dụng** — 7 vendor còn lại có 0 file, không đủ căn cứ thật để
port theo đúng nguyên tắc dự án (luôn đối chiếu với Python thật trên dữ
liệu thật).

**⚠️ Rủi ro xuyên suốt thiết kế này: chỉ có 1 đơn hàng thật duy nhất để
kiểm chứng.** Không có sản phẩm nhiều dòng tên, không có biến thể layout
khác, không có trường hợp OU Qty/giá bất thường để đối chiếu. Thiết kế
dưới đây trung thực với những gì ĐÃ xác minh được qua file mẫu này — bất
kỳ giả định nào vượt ra ngoài bằng chứng thật đều được đánh dấu rõ.

### Kế thừa từ các phase trước

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ".
Cùng hạ tầng dùng chung: `excelwriter.Row`/`WriteOrderRows`,
`pricing.Index`/`FetchIndex`, `processor_shared.go`'s
`regionInfo`/`closeEnough`/`buildPromoBonusRow`/`buildInvoiceBonusRow`/
`coopDebtDays`.

## Phát hiện kiến trúc quan trọng: JMart dùng CHUNG hàm ghi Excel với Kingfood

Xác nhận trực tiếp qua `xulydonhang.py:8143-8209` (nhánh
`elif vendor == "JMart":` trong `process_file`): JMart gọi thẳng
```python
saigia = ProcessHandler.write_to_dondathang_kingfood(self,products,makhachhang,po_number,entry_date,cancel_date,stt,vendor,delivery_address,file_url)
```
— **không có `write_to_dondathang_jmart` riêng**. `makhachhang` luôn là
hằng số cứng `'MN_MT_JM0001'` (dòng 8144) — trùng khớp CHÍNH XÁC với
nhánh đặc biệt đã có sẵn trong Go's `kingfoodRegionInfo`
(`kingfood_processor.go`, `case customerCode == "MN_MT_JM0001": return
"MT_MN", "LA", "LA_TP"`) — nhánh này viết cho Kingfood's kiến trúc nhưng
CHƯA từng có input thật nào chạm tới; JMart chính là vendor khiến nhánh
đó trở thành có ý nghĩa trên thực tế.

**Quyết định thiết kế**: `processJMartSegment` (file mới) viết logic ghi
dòng Excel theo đúng khuôn `processKingfoodSegment` (khuyến mãi từng sản
phẩm không ghi AP khi fallback, khuyến mãi cấp hóa đơn `Q=SKU đầu tiên`,
AU ghi bình thường, không skip giá 0) — **gọi thẳng hàm
`kingfoodRegionInfo(customerCode)` đã có sẵn** (cùng package `processing`,
không cần sửa/thêm gì vào Kingfood's code đã ship) thay vì viết
`jmartRegionInfo` riêng hay refactor `processKingfoodSegment` thành hàm
dùng chung — tái dùng an toàn (chỉ GỌI hàm đã có, không đụng vào code đã
test/ship), theo đúng tinh thần dè dặt khi đụng vào code production đã
hoạt động.

## Dữ liệu đối chiếu (golden corpus)

**1 file PDF JMart thật khả dụng** (tại thời điểm viết spec):
```
đơn hàng/mẫu đơn hàng/13-07-2026/13-07-2026_[JMart][05-07-2026][MN_MT_JM0001][05-07-2026][DH01010844].pdf
```
3 sản phẩm, 1 trang.

**Nhận diện** (`identify_vendor`, `xulydonhang.py:145-146`):
```python
if re.search(r"Đơn vị : HỆ THỐNG SIÊU THỊ JMART", cleaned_text):
    return "JMart"
```
Marker chuỗi đơn giản, không có `or`. Xác nhận có trong file mẫu thật.

**Vị trí trong `identify_vendor`'s thứ tự kiểm tra**: ...FujiMart(128) →
Tiktok(131) → KOC(134,139) → JMart(145) → MR.DIY(149) → BHX(152) → BC
Mart(155) → Farmer(159) → JV-Mart(162) → ... Vì Tiktok/KOC (3 vendor đứng
giữa FujiMart và JMart trong Python) đều **chưa port**, case JMart trong
Go's `vendor.Identify` chỉ cần **nối sau FujiMart** (case cuối cùng hiện
có) — không cần chèn giữa, khác Kingfood (vendor gần nhất cần chèn giữa).

**Xác nhận: JMart là "1 trang = 1 đơn"** — cùng nhóm đa số vendor đã port.

## ⚠️ Lỗi nghiêm trọng nếu port thẳng `tachsanpham_JMart` — đã phát hiện qua Task 0 smoke test

Chạy trực tiếp Go's `extractPageTexts` trên file mẫu thật, SONG SONG với
chạy trực tiếp hàm Python thật (`cat_giua_theo_dong` +
`tachsanpham_JMart`) trên CÙNG file — phát hiện 2 pipeline cho ra hình
dạng text **khác nhau đáng kể**, ngược hướng với hầu hết các trường hợp
đã gặp trước đây trong dự án (thường Go tách dòng NHIỀU hơn Python; ở
đây Python tách dòng NHIỀU hơn Go):

**PyMuPDF (Python) — bị PDF's độ rộng cột ép tách dòng giữa token**:
```
ST
T
...
133,806.
000
8.000
1.00
0
Gói
...
NƯỚC GIẶT XẢ BLUE ĐẬM
ĐẶC H. NƯỚC HOA 3.6 L
8936156730886
```

**Go's `extractPageTexts` — SẠCH, không tách giữa token**:
```
STT
...
133,806.000
8.000
1.000
Gói
...
NƯỚC GIẶT XẢ BLUE ĐẬMĐẶC H. NƯỚC HOA 3.6 L
8936156730886
```

`tachsanpham_JMart`'s thuật toán tìm số lượng (OU Qty) dựa vào tìm dòng
**KHỚP CHÍNH XÁC chuỗi `"1.00"`** (`xulydonhang.py:6962`) — đây thực chất
là hệ quả của việc PyMuPDF cắt `"1.000"` (giá trị cột QC/quy cách, LUÔN
LÀ `1.000` trong dữ liệu thật) thành 2 dòng `"1.00"` + `"0"`. **Nếu port
thẳng chuỗi `"1.00"` sang Go, code sẽ KHÔNG BAO GIỜ khớp** vì Go's text
giữ nguyên `"1.000"` trên 1 dòng — `OU Qty` sẽ luôn `nil`, gây crash hoặc
lỗi âm thầm khi ghi Excel.

**Đã xác minh bằng cách chạy TRỰC TIẾP hàm Python thật** (không chỉ đọc
code) trên file mẫu — kết quả thật:
```python
[{'Barcode': '8936156730886', 'OU Qty': '8', 'Total Price': '133806.000'},
 {'Barcode': '8936156732668', 'OU Qty': '12', 'Total Price': '26836.000'},
 {'Barcode': '8936156732675', 'OU Qty': '12', 'Total Price': '26836.260'}]
```

**Đối chiếu ngược lại với Go's text sạch, xác nhận vị trí dòng chính xác
tương ứng với từng trường** (tính từ dòng barcode, đếm lùi — thứ tự cột
trong bảng PDF, từ dưới lên: STT, Mã vật tư, Barcode, Tên đầy đủ, Tồn
kho, ĐVT, QC, Số lượng, Đơn giá, Chiết khấu, Thành tiền):
- `Barcode` = dòng chứa đúng 13 chữ số.
- `Tên đầy đủ` (bỏ qua, không dùng) = dòng ngay trước Barcode.
- `Tồn kho` (bỏ qua) = 2 dòng trước Barcode.
- `ĐVT` (bỏ qua) = 3 dòng trước Barcode.
- **`QC` = 4 dòng trước Barcode, LUÔN LÀ `"1.000"` trong dữ liệu thật đã
  xác minh** — dùng làm điểm neo (anchor), y hệt vai trò của `"1.00"`
  trong Python, chỉ khác giá trị cần khớp.
- **`Số lượng` (→ OU Qty) = dòng NGAY TRƯỚC dòng QC** (tức 5 dòng trước
  Barcode) — khớp đúng logic Python "lấy dòng trước dòng `1.00`", chỉ
  khác điểm neo.
- **`Đơn giá` (→ Total Price) = dòng khớp pattern giá quốc tế
  `\d{1,3}(,\d{3})+\.\d{3}` (dấu phẩy=nghìn, dấu chấm=thập phân, y hệt
  chuẩn quốc tế, KHÁC Kingfood's định dạng Việt Nam đảo ngược) tìm được
  ĐẦU TIÊN khi quét lùi từ Barcode** — phần này **port thẳng được** từ
  Python (`price_pattern = r'\d{1,3}(?:,\d{3})+\.\d{3}'`), vì nó dựa vào
  ĐỊNH DẠNG SỐ (yêu cầu có dấu phẩy) chứ không dựa vào lỗi tách dòng của
  PyMuPDF — hoạt động đúng y hệt trên cả 2 pipeline. Xác nhận: `Chiết
  khấu` (`"0"`, không có dấu phẩy) và `QC`/`Số lượng` (không có dấu phẩy)
  đều KHÔNG khớp pattern này, nên quét lùi không dừng nhầm ở đó — chỉ
  `Đơn giá` (`"133,806.000"`) mới khớp.

**Quyết định thiết kế**: viết lại thuật toán quét lùi cho Go dựa trên
CHÍNH điểm neo `"1.000"` (không phải `"1.00"`), lấy dòng ngay trước làm
`OU Qty` — port trực tiếp phần tìm `Total Price` (dựa định dạng số, hoạt
động đúng trên cả 2 pipeline). **Không port** 2 dòng `re.sub` chuẩn hoá
số bị tách dòng của Python (`xulydonhang.py:6942-6943`) — chúng chỉ tồn
tại để sửa lỗi tách dòng CỦA RIÊNG PyMuPDF, không xảy ra trong Go's text
(xác nhận qua Task 0 — không cần port hành vi Python không có ý nghĩa
trong Go).

**Chưa xác minh được (do chỉ có 1 mẫu)**: liệu offset "5 dòng trước
Barcode" cho `Số lượng` có ổn định với mọi đơn hàng thật khác hay không
(ví dụ nếu `Tên đầy đủ` xuống dòng do quá dài). Thiết kế dùng "khớp neo
`1.000` rồi lấy dòng trước" (không phải offset cố định tuyệt đối tính từ
Barcode) để giảm rủi ro này — nếu `Tên đầy đủ`/`ĐVT`/`Tồn kho` có xuống
dòng thêm, khoảng cách neo-tới-Barcode sẽ thay đổi nhưng khoảng cách
neo-tới-Số-lượng (luôn là 1 dòng ngay trước) vẫn đúng — miễn `Số lượng`
không tự nó xuống dòng (không có bằng chứng cho thấy nó có, dựa trên mẫu
duy nhất).

## Trích xuất PO/ngày/địa chỉ (`xulydonhang.py:8146-8153`)

```python
entry_date = re.search(r"Ngày in\s*:\s*(\d{1,2}/\d{1,2}/\d{4})", text).group(1)
cancel_date = entry_date  # LUÔN bằng entry_date, không có logic riêng
po_number = re.search(r"Số phiếu đặt\s*:\s*([A-Z0-9]+)", text).group(1)
m = re.search(r"Địa chỉ giao hàng\s*:\s*(.+?)\s*SĐT nhận hàng\s*:", text, re.S)
delivery_address = m.group(1).strip() if m else None
```

Xác nhận cả 3 marker (`"Ngày in :"`, `"Số phiếu đặt:"`, `"Địa chỉ giao
hàng:"..."SĐT nhận hàng :"`) xuất hiện Y HỆT NHAU giữa PyMuPDF và Go's
text (không bị tách dòng khác nhau ở khu vực này, khác khu vực bảng sản
phẩm) — regex Python port THẲNG được sang Go, không cần kỹ thuật quét
dòng chịu lỗi như các vendor khác.

**`cancel_date = entry_date`** — không có logic cross-validate/fallback
nào (giống Kingfood), và đặc biệt đây là gán trực tiếp bằng nhau (không
qua parse lại) — port y nguyên: `cancelDate := entryDate`.

**Không có `.group(1)` guard nào (không try/except)** — nếu marker không
tìm thấy, Python crash thật (`AttributeError: 'NoneType' object has no
attribute 'group'`). Go port: `ok=false` sạch nếu bất kỳ marker nào
không khớp, theo đúng chính sách dự án.

`re.S` (DOTALL) cho địa chỉ giao hàng — Go dùng `(?s)` flag tương đương.
Xác nhận qua Go's text thật: `"Địa chỉ giao hàng:\nL1..."` (label và giá
trị CÙNG DÒNG trong Python's raw text nhưng TÁCH DÒNG trong Go's — do
`(?s)` khiến `.` khớp cả `\n`, regex vẫn hoạt động đúng bất kể cách nào,
không cần xử lý đặc biệt).

## Cắt vùng bảng sản phẩm — `cat_giua_theo_dong` (`xulydonhang.py:6177-6205`)

Hàm cắt dòng TỔNG QUÁT (không riêng JMart, tham số hoá `dau_line`/
`cuoi_line`): tìm dòng đầu tiên BẮT ĐẦU BẰNG `dau_line` (không cần khớp
chính xác cả dòng), lấy mọi dòng SAU đó cho đến dòng KHỚP CHÍNH XÁC
`cuoi_line`. JMart gọi với `dau_line="Mã vật tư"`, `cuoi_line="Tổng:"`.

Xác nhận cả 2 marker xuất hiện y hệt (dòng riêng, không bị cắt) trong cả
2 pipeline. Go port: hàm quét dòng tương đương, `strings.HasPrefix` cho
điểm bắt đầu, so khớp chính xác cho điểm kết thúc.

## Cột ghi (khớp `write_to_dondathang_kingfood`, xem
`docs/superpowers/specs/2026-08-19-kingfood-real-processor-design.md`
để biết chi tiết đầy đủ từng khối) — chỉ nêu điểm khác biệt cụ thể cho
JMart:

- `vendor` (uppercase trong order number `ĐĐH{VENDOR}-{po}`) = `"JMART"`.
- `makhachhang` = hằng số cứng `"MN_MT_JM0001"`.
- `delivery` = trích từ PDF thật (khác Kingfood's hằng số cứng) — dùng
  `deliveryAddress` trích được, KHÔNG có giá trị mặc định fallback nào
  trong Python (`delivery_address = m.group(1).strip() if m else None` —
  nếu `None`, Python ghi `None` trực tiếp vào ô Excel qua
  `f"{current_row}"] = delivery` — hành vi kỳ lạ nhưng đây là Python
  thật; Go port: nếu marker địa chỉ không khớp, coi là `ok=false` luôn
  (nhất quán với chính sách "không port hành vi lỗi ngầm", và vì đằng
  nào `po_number`/`entry_date` cũng phải khớp trước đó theo cùng khối
  regex không try/except).
- `Total Price` (Đơn giá) = giá ĐƠN VỊ trực tiếp (không nhân/chia gì) —
  giống hệt Kingfood's cách dùng `giahoadon = dongia =
  float(product["Total Price"])` không chia cho số lượng.

## Phạm vi

### Làm thật
- Package mới `GO/internal/processing/jmart/`: `ParseOrderInfo`
  (PO/ngày/địa chỉ, port thẳng regex vì không có lệch layout ở khu vực
  này), `ExtractProducts` (cắt bảng qua `cat_giua_theo_dong`-tương đương
  + thuật toán quét lùi ĐÃ ĐIỀU CHỈNH cho Go's text sạch, neo `"1.000"`
  thay vì `"1.00"`).
- `jmart_processor.go`: `processJMartSegment`, gọi thẳng
  `kingfoodRegionInfo("MN_MT_JM0001")` (không viết hàm mới, không sửa
  Kingfood's code), tái dùng `buildPromoBonusRow`/`buildInvoiceBonusRow`
  y hệt khuôn Kingfood.
- Chèn `vendor.Identify`'s case JMart — nối sau FujiMart (case cuối).
- Copy 1 PDF thật vào `jmart/testdata/realpdfs/` (cần hỏi lại chủ dự án
  — không giả định giống các vendor trước) + sinh 1 fixture qua
  `generate_fixtures.py`.
- Golden test với ĐÚNG 1 fixture — ghi rõ trong comment rằng độ phủ rất
  hạn chế, không đại diện cho mọi biến thể đơn hàng JMart thật.

### Không làm
- Không tìm cách bổ sung dữ liệu mẫu giả — chỉ dùng 1 file thật đã có.
- Không port 2 dòng `re.sub` chuẩn hoá số bị tách dòng (không cần thiết
  cho Go's text, xem mục "Lỗi nghiêm trọng" ở trên).
- Không refactor `processKingfoodSegment` thành hàm dùng chung — chỉ gọi
  `kingfoodRegionInfo` trực tiếp (rủi ro thấp nhất cho code đã ship).
- Upload PDF lên Google Drive.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go              # + case JMart, NỐI SAU FujiMart (case cuối)
  jmart/
    extract.go                # ParseOrderInfo, ExtractProducts (quét lùi
                              # neo "1.000")
  jmart_processor.go          # processJMartSegment (gọi kingfoodRegionInfo)
  jmart_processor_test.go
  jmart_golden_test.go        # CHỈ 1 fixture — ghi rõ giới hạn độ phủ

GO/internal/processing/jmart/testdata/
  realpdfs/DH01010844.pdf     # 1 file duy nhất, cần xác nhận commit git
  fixtures/DH01010844.json
  generate_fixtures.py
```

## Dữ liệu ngoài & phụ thuộc mạng

- **`settings.ini`**: đã xác nhận có sẵn gid cho `JMART`
  (`JMART = 1522007492`), không cần sửa.
- Không cần fuzzy-match customer-code (hardcode `MN_MT_JM0001`).

## Testing

- Unit test `ParseOrderInfo`/`ExtractProducts` dùng dữ liệu thật đã xác
  minh qua cả 2 pipeline (bao gồm case quét lùi neo `"1.000"`).
- Test riêng xác nhận `Total Price` parse đúng định dạng quốc tế (dấu
  phẩy=nghìn, dấu chấm=thập phân) — KHÁC Kingfood, dùng được
  `parseNumericField` dùng chung trực tiếp (không cần hàm parse riêng
  như `parseKingfoodPrice`).
- Golden fixture: chỉ 1 file — test PASS ở đây có giá trị xác minh hạn
  chế, cần ghi rõ trong comment code.

## Rủi ro / lưu ý

- **Toàn bộ thiết kế dựa trên ĐÚNG 1 đơn hàng thật.** Offset "5 dòng
  trước Barcode = Số lượng" (neo qua QC `"1.000"`) chưa được xác minh ổn
  định qua nhiều đơn hàng — nếu có thêm PDF JMart thật trong tương lai,
  cần chạy lại Task 0 smoke test và đối chiếu golden fixture trước khi
  tin tưởng thiết kế này hoàn toàn đúng cho mọi trường hợp.
- **`delivery_address = None` khi không khớp marker** — Python's hành vi
  thật ghi `None` vào Excel nếu marker không tìm thấy; Go port chọn
  `ok=false` sạch thay vì tái tạo hành vi này, cần ghi vào
  `knownDivergences_JMart` nếu golden fixture cho thấy khác biệt cụ thể.
- **Quyết định commit PDF thật vào git** cần hỏi lại chủ dự án riêng cho
  JMart, không tự động áp dụng quyết định đã có cho các vendor trước.
