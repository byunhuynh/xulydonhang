# Thiết kế: RealProcessor cho Lotte (Phase 2b-1 của dự án refactor)

## Bối cảnh

Phase 2a (đã hoàn tất, xem
[2026-08-14-coop-real-processor-design.md](2026-08-14-coop-real-processor-design.md))
thay `MockProcessor` bằng `RealProcessor` xử lý thật cho vendor Coop, đạt
93/155 file mẫu khớp hoàn toàn với bản Python — 62 file còn lại là giới hạn
trích xuất text PDF, đã ghi nhận là giới hạn chấp nhận được của Phase 2a,
không thuộc phạm vi tài liệu này.

Người dùng vừa bổ sung thêm 153 file PDF mẫu thật vào
`đơn hàng/08-2026/`, thuộc **7 nhà cung cấp mới** ngoài Coop. Phân loại tự
động (dùng đúng logic `identify_vendor` của Python) cho kết quả:

| Nhà cung cấp | Số file mẫu |
|---|---|
| Lotte | 60 |
| Satra | 33 |
| BigC | 27 |
| Winmart | 16 |
| Emart | 9 |
| FujiMart | 4 |
| Kingfood | 3 |

Đây là khối lượng công việc lớn hơn nhiều so với dự kiến ban đầu (chỉ tính
BigC) trong spec Phase 2a. Theo quyết định của người dùng, mỗi nhà cung cấp
là **một sub-project riêng** (spec + plan + bộ golden-fixture riêng), làm
tuần tự theo thứ tự số lượng file mẫu giảm dần — Lotte trước tiên vì có
nhiều dữ liệu mẫu nhất, sau đó Satra → BigC → Winmart → Emart → FujiMart →
Kingfood. Tài liệu này là spec cho **Lotte**, sub-project đầu tiên của
Phase 2b.

### Khác biệt chính sách so với Phase 2a (quan trọng)

Phase 2a bắt buộc **giữ nguyên mọi bug** của bản Python để đảm bảo tương
thích hành vi 100%. Với Phase 2b, người dùng đã chọn chính sách khác:
**làm đúng luồng chính, không bắt buộc giữ bug cũ.** Điều này chỉ ảnh
hưởng đến quy tắc kiểm chứng (xem mục "Chiến lược kiểm chứng" bên dưới),
không thay đổi kiến trúc.

## Dữ liệu đối chiếu (golden corpus)

**60 file PDF Lotte thật** tại `đơn hàng/08-2026/*.pdf` (đã có sẵn, không
cần thu thập thêm) — nhận diện qua `identify_vendor`'s Lotte pattern
(`0107889783\s*009333` hoặc `1102018142\s*010544` trong text đã chuẩn hóa
khoảng trắng).

### Chiến lược kiểm chứng

Giống Phase 2a về cơ chế, khác về tiêu chí chấp nhận sai lệch:

1. Viết script Python throwaway (`GO/internal/processing/lotte/testdata/generate_fixtures.py`,
   không phải phần của ứng dụng) gọi trực tiếp đoạn xử lý Lotte trong
   `process_file` (`xulydonhang.py:9079-9139`, cùng các hàm phụ thuộc:
   `tachcancledate_lotte`, `tachsanpham_lotte`, `laytenstore_lotte`,
   `get_makhachhang_lotte`, `write_to_dondathang_lotte`) trên cả 60 file,
   không qua GUI, ghi kết quả từng field ra JSON — một fixture/file, đúng
   hình dạng JSON đã dùng ở Phase 2a.
2. Đóng băng dữ liệu giá/khuyến mãi Lotte lấy từ Google Sheets (gid
   `LOTTE` trong `settings.ini`, đã có sẵn: `435921079`) tại thời điểm
   sinh fixture, lưu kèm trong JSON — test Go không phụ thuộc mạng.
3. Test Go đọc từng file PDF + fixture JSON, chạy `RealProcessor` (đã mở
   rộng dispatch sang Lotte), so khớp từng field.
4. **Khác Phase 2a:** nếu Go tính khác Python trên một field cụ thể, **và
   có thể xác nhận Python tính sai** (đối chiếu tay với nội dung PDF gốc
   — ví dụ số lượng/tổng tiền không khớp phép tính cơ bản, ngày tháng vô
   lý, sản phẩm bị bỏ sót rõ ràng do lỗi regex Python), sai lệch đó được
   **ghi chú rõ trong code (comment) và trong ledger khi triển khai**,
   không bắt buộc phải làm Go sai theo. Fixture cho case đó vẫn giữ giá
   trị gốc của Python (để làm bằng chứng lịch sử), nhưng test so khớp cho
   field/file đó được đánh dấu ngoại lệ có tài liệu, không phải xóa hay
   sửa fixture. Mọi sai lệch khác (không xác nhận được Python sai) vẫn là
   bug Go cần sửa như bình thường.

## Phạm vi

### Làm thật

- Trích xuất Lotte đầy đủ, từng trang PDF độc lập (giống Coop, KHÔNG có
  bước đếm/tách nhiều đơn trên 1 trang như Coop — quan sát trong Python:
  mỗi trang Lotte luôn là đúng 1 đơn hàng, đơn giản hơn Coop ở điểm này):
  - Số PO + ngày đặt hàng: 2 dòng đầu trang, ghép định dạng
    `yyMMdd-storecode-order` rồi tách `time_part`/`store_code`/`order_number`
    (`xulydonhang.py:9081-9092`).
  - Ngày hủy đơn: các dòng ngày tháng nằm giữa dòng chứa PO# và dòng
    `"00:00"` (`tachcancledate_lotte`, `:6051-6071`).
  - Tên cửa hàng: dòng cuối cùng trước PO# nằm sau mốc `"DOAN TUAN ANH"`
    (`laytenstore_lotte`, `:6565-6584`).
  - Danh sách sản phẩm: cắt khối text giữa `"Sply qty"` và
    `"Tot add tax"` (`lamsachdonhang_lotte`, `:6405-6423`), rồi áp regex
    cố định lấy mã hàng/barcode/số lượng/thành tiền
    (`tachsanpham_lotte`, `:6074-6091`).
  - Mã khách hàng: tra `data.xlsx` sheet `MaKH`, lọc cột A = `"LOTTE"`,
    khớp *hậu tố* cột C với store code (`get_makhachhang_lotte`,
    `:307-321` — khác cách khớp của Coop, không có bug double-read cột
    B/C như Coop).
  - Tính giá/khuyến mãi thực tế và cờ sai giá: **tái dùng nguyên vẹn**
    logic đã port ở Phase 2a (`pricing.Index.FindPrice`/`FindPromotions`,
    cùng công thức `math.isclose(rel_tol=1e-4)`, cùng quy tắc tô đỏ +
    comment) — xác nhận đối chiếu Python cho thấy đoạn này của
    `write_to_dondathang_lotte` (`:2083-2186`) giống hệt logic Coop, chỉ
    khác đầu vào (barcode/vendor="LOTTE").
- Ghi kết quả ra **file Excel riêng để test** (không phải `dondathang.xlsx`
  thật), tái dùng nguyên `excelwriter.Row`/`WriteOrderRows` — đã xác nhận
  layout cột giống 100% Coop (cùng sheet "Don dat hang", cùng cột
  A/B/C/D/E/G/L/Q/S/T/U/V/X/Y/Z/AE/AJ/AM/AT/AU/AV), chỉ khác **giá trị**
  từng vendor tự tính (`kho`/`khuvuc`/`mien` của Lotte rẽ nhánh theo store
  code bắt đầu bằng `"MB"` hay không, khác nhánh của Coop).
- Mở rộng `vendor.Identify` nhận diện Lotte.
- Mở rộng dispatch của `RealProcessor.Process` (điểm mở rộng đã có sẵn từ
  Phase 2a — xem `GO/internal/processing/coop_processor.go:34` comment
  "support for other vendors is added in later phases by extending this
  same dispatch") sang nhánh Lotte, gọi package `lotte` mới.
- Tổng quát hóa `pricing.HTTPSource.FetchCoopIndex()` (hiện hardcode gid
  `"COOP"`) thành nhận tham số tên sheet (vd `FetchIndex(sheetKey string)`),
  vì cùng 1 Google Sheet ID dùng chung cho mọi vendor, chỉ khác gid theo
  tab — đã xác nhận `settings.ini` có sẵn `LOTTE = 435921079`. Đây là thay
  đổi nhỏ, không phá vỡ Coop (gọi `FetchIndex("COOP")` thay vì
  `FetchCoopIndex()`).
- Thêm hàm tra cứu mã khách hàng kiểu Lotte vào `productdata.Store` (khớp
  hậu tố, lọc theo tên hệ thống) — khác `GetCustomerCode` hiện có (dành
  riêng cho bug cụ thể của Coop), không sửa hàm cũ.
- Mọi trường hợp bất thường (không tách được PO#, không tìm được sản
  phẩm, mã khách hàng không tồn tại...) phải hiện thành `OrderRow` trạng
  thái Thất bại kèm lý do cụ thể — theo đúng nguyên tắc đã lập ở Phase 2a.

### Không làm (ngoài phạm vi Phase 2b-1)

- Satra, BigC, Winmart, Emart, FujiMart, Kingfood và mọi vendor khác —
  từng cái là sub-project riêng sau, theo thứ tự đã thống nhất.
- Upload PDF lên Google Drive (`upload_file_to_drive`, gọi ở
  `xulydonhang.py:9113/9122`) — cùng lý do đã loại ở Phase 2a, để dành
  làm cùng đợt automation Zalo/MISA.
- Ghi trực tiếp vào `dondathang.xlsx` thật ở gốc repo.
- Zalo notification liên quan Lotte (`MNLOTTE`/`MBLOTTE` trong
  `settings.ini`'s `<zalo>` block) — thuộc nhóm automation ngoài, chưa
  làm ở phase này.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go          # + pattern nhận diện Lotte:
                          # "0107889783\s*009333" hoặc
                          # "1102018142\s*010544"
  lotte/
    extract.go            # ParseOrderInfo(text) — PO#, entry date từ
                           # 2 dòng đầu trang
                           # ExtractCancelDate(text, poNumber) string
                           # ExtractStoreName(text, poNumber) string
                           # ExtractProducts(text) ([]Product, error)
    linesbetween.go        # LinesBetween(text, startPattern, endMarker
                           # string) []string — helper dùng chung cho cả
                           # 3 hàm trích xuất kiểu "tìm dòng bắt đầu X,
                           # dừng ở dòng Y" (cancel date, tên cửa hàng,
                           # danh sách sản phẩm) — cân nhắc đưa lên gói
                           # dùng chung (vd package con của processing)
                           # nếu Satra/vendor sau cũng cần đúng pattern
                           # này, quyết định cụ thể để dành cho lúc viết
                           # plan.
  productdata/
    store.go               # + GetCustomerCodeBySuffix(system, storeCode
                           # string) string — mirror get_makhachhang_lotte
  pricing/
    http_source.go          # FetchCoopIndex() đổi thành
                           # FetchIndex(sheetKey string) (*Index, error);
                           # Index/ParseIndex/FindPrice/FindPromotions/
                           # FindInvoicePromotion giữ nguyên, đã vendor-
                           # agnostic sẵn
  excelwriter/
    dondathang.go           # không đổi — Row/WriteOrderRows tái dùng
                           # nguyên vẹn
  lotte_processor.go         # xử lý per-page cho nhánh Lotte, gọi từ
                           # RealProcessor.Process (mở rộng nhánh
                           # `if v != "Coop"` hiện tại thành dispatch
                           # theo vendor cụ thể)
  lotte_golden_test.go        # đối chiếu 60 fixture JSON

GO/internal/processing/lotte/testdata/
  fixtures/*.json             # sinh bởi script Python, đã đóng băng giá/KM
  generate_fixtures.py         # script throwaway
```

### Ánh xạ trạng thái OrderRow

Tái dùng nguyên `StatusDone/StatusWarning/StatusFailed` — cùng ngữ nghĩa
đã định nghĩa ở Phase 2a (Done = khớp giá, Warning = ghi được nhưng lệch
giá đã tô đỏ, Failed = không ghi được dòng, lý do cụ thể trong `Status`).

## Dữ liệu ngoài & phụ thuộc mạng

- **`data.xlsx`**: sheet `MaKH` — thêm truy vấn lọc theo cột A =
  `"LOTTE"`, khớp hậu tố cột C. Không cần thay đổi sheet `SanPham`
  (`productdata.GetProductInfo`/`ResolveSku` dùng chung, barcode Lotte là
  số 12-13 chữ số, không phải mã 7 chữ số kiểu Coop — cần xác nhận cách
  `load_sku_mapping`/`replace_sku_numbers` xử lý mã dài hơn khi viết
  plan).
- **`settings.ini`**: `<gid>` block đã có sẵn `LOTTE = 435921079`. Đã xác
  nhận trong `xulydonhang.py` (`find_price_by_sku:5584-5610`,
  `laygiathucte_CNHCM:5616` và 2 hàm khác cùng khối) rằng **mọi vendor
  dùng chung một `sheet_id` hardcode**
  (`1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4`), chỉ `gid` khác theo
  tên sheet truyền vào (`sheet_name` param, mặc định `"COOP"`) — nên việc
  tổng quát hóa `FetchCoopIndex()` → `FetchIndex(sheetKey string)` chỉ
  cần đổi gid, giữ nguyên `spreadsheetID` hằng số hiện có.
- **Google Drive upload / Zalo**: không gọi ở Phase 2b-1 (xem mục Không
  làm).

## Testing

- Go unit test cho từng hàm trích xuất thuần (`lotte/extract.go`,
  `lotte/linesbetween.go`) theo TDD, cùng phong cách Phase 2a.
- `lotte_golden_test.go`: test tích hợp chạy `RealProcessor` trên 60
  fixture, dùng `PricingSource` đã đóng băng — lưới an toàn chính. Áp
  dụng chính sách ngoại lệ có tài liệu đã mô tả ở mục "Chiến lược kiểm
  chứng" cho các field mà Go cố ý sửa đúng chỗ Python sai.
- `productdata`/`excelwriter` tiếp tục dùng bản sao `data.xlsx`/
  `dondathang.xlsx` mẫu trong `testdata/`.

## Rủi ro / lưu ý

- `laytenstore_lotte` dựa vào chuỗi cố định `"DOAN TUAN ANH"` (tên người
  liên hệ cụ thể xuất hiện trên mọi PO Lotte theo mẫu hiện tại) làm mốc
  định vị — heuristic dễ vỡ nếu Lotte đổi mẫu PO trong tương lai, giữ
  nguyên vì đó là cách Python đang làm và có dữ liệu mẫu xác nhận hoạt
  động trên 60 file.
- Barcode Lotte dài 12-13 chữ số, khác hẳn định dạng `1234567-1` của
  Coop — `productdata.CleanSkuNumber`/`ResolveSku` hiện được viết riêng
  cho định dạng Coop; cần xác nhận khi viết plan xem có áp dụng được cho
  Lotte hay cần đường dẫn tra cứu riêng.
- Áp dụng đúng nguyên tắc đã có ở Phase 2a: file PDF không tách được PO#
  hoặc không tìm được sản phẩm sẽ hiện dòng "Thất bại" thay vì bị bỏ qua
  âm thầm — người dùng cần biết trước khi thấy nhiều dòng đỏ hơn so với
  không thấy gì ở bản Python cũ.
