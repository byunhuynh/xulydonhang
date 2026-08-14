# Thiết kế: RealProcessor cho Coop (Phase 2a của dự án refactor)

## Bối cảnh

Phase 1 (đã hoàn tất, xem
[2026-08-13-go-wails-skeleton-design.md](2026-08-13-go-wails-skeleton-design.md))
dựng khung sườn Go + Wails + React với `MockProcessor` giả lập xử lý đơn
hàng. Interface `processing.Processor` được thiết kế sẵn một seam để cắm
logic xử lý thật vào mà không cần sửa UI hay event contract.

Phase 2 thay `MockProcessor` bằng engine parse PDF thật, bắt đầu với
**một vendor duy nhất: Coop** (Co-opmart/Co-opfood). BigC và các vendor
còn lại (~18 vendor khác trong `xulydonhang.py`) là các sub-project riêng
sau, mỗi cái có spec/plan của nó — không nằm trong tài liệu này.

### Vì sao chỉ Coop, không làm cùng BigC

Khảo sát `xulydonhang.py` cho thấy Coop và BigC có cấu trúc PDF và luồng xử
lý rất khác nhau (Coop: mỗi trang PDF là 1 hoặc nhiều đơn hàng hoàn chỉnh,
tách bằng đếm từ khóa; BigC: dàn trải theo **vị trí trang** — trang đầu
chứa bảng giá tổng, mỗi trang giữa là một cửa hàng, ghi Excel theo cách gọi
hàm khác nhau tùy vị trí trang). Coop có nhiều dữ liệu mẫu thật hơn (155
file so với 27 của BigC, xem bên dưới) và là vendor đơn giản hơn để làm
trước, giảm rủi ro khi thiết lập pattern lần đầu.

## Dữ liệu đối chiếu (golden corpus)

Repo đã có sẵn **155 file PDF Coop thật** tại `đơn hàng/08-2026/*.pdf`
(được app Python hiện tại tự động lưu lại sau khi xử lý thành công — tên
file là số PO). Không cần thu thập thêm.

### Chiến lược kiểm chứng

Vì phần lõi (`extract_products`) là heuristic dò vị trí số liệu theo layout
text của PDF — không phải parse có ngữ pháp rõ ràng — không thể chỉ đọc
code Python rồi suy luận đúng/sai. Bắt buộc phải đối chiếu với hành vi thật
hiện tại:

1. Viết một script Python độc lập (`GO/internal/processing/coop/testdata/generate_fixtures.py`
   hoặc tương đương, nằm ngoài `xulydonhang.py`) gọi trực tiếp
   `process_coop_invoice` (và các hàm nó cần) trên cả 155 file PDF, **không
   qua GUI**, ghi kết quả mỗi đơn (mọi field sẽ ghi vào Excel — PO, ngày,
   sản phẩm, số lượng, đơn giá, ghi chú, có sai giá hay không...) ra file
   JSON, một file fixture cho mỗi PDF.
2. Tại thời điểm sinh fixture, **đóng băng** luôn dữ liệu giá/khuyến mãi
   Coop lấy được từ Google Sheets (lưu kèm trong JSON) — để test Go sau
   này không phụ thuộc mạng và không bị "flaky" nếu sheet thay đổi.
3. Test Go (`coop_processor_test.go`) đọc từng file PDF + fixture JSON
   tương ứng, chạy `RealProcessor`, so khớp từng field. Bộ pricing dùng
   trong test Go phải nạp từ dữ liệu đã đóng băng trong fixture (không gọi
   Google Sheets thật khi chạy `go test`).
4. Bất kỳ sai lệch nào giữa Go và fixture đều là bug cần sửa trước khi coi
   là xong — không có khái niệm "gần đúng" cho dữ liệu kế toán.

Script sinh fixture ở bước 1 là **throwaway/dev-tool**, không phải một
phần của ứng dụng Go, chạy một lần (hoặc lại khi cần refresh fixture), kết
quả JSON được commit vào `testdata/` để test Go chạy offline, lặp lại
được.

## Phạm vi

### Làm thật

- Trích text PDF bằng thư viện Go thuần `github.com/ledongthuc/pdf`
  (không cần CGO/trình biên dịch C — máy dev hiện chưa có gcc, đã xác minh
  thư viện này đọc đúng file Coop thật: nhận diện được `POM343`,
  `P/O Number`, mã SKU dạng `1234567-1`, `Sub Total`, tiếng Việt không lỗi
  font, trên file mẫu thật `102945235-00.pdf`).
- Toàn bộ luồng Coop: đếm số đơn/trang, tách nhiều đơn/trang, trích thông
  tin đơn (PO#, địa điểm, ngày nhận/hủy, ghi chú, nơi giao), trích danh
  sách sản phẩm (heuristic dò số lượng/đơn giá theo khối văn bản giữa các
  mã SKU), tách text khuyến mãi theo quy ước `cm`/`cf`, tra cứu SKU nội bộ,
  tra cứu tên/trọng lượng/quy cách sản phẩm.
- Tra cứu khách hàng/hệ thống (COOPMART/COOPFOOD) từ `data.xlsx` — nạp
  **một lần** vào bộ nhớ khi khởi động RealProcessor, không đọc lại file
  mỗi lần tra cứu như bản Python.
- Tra cứu giá + khuyến mãi Coop từ Google Sheets — tải **một lần mỗi đơn
  hàng** (toàn bộ sheet giá của Coop qua URL CSV export có sẵn từ
  `settings.ini`), tra cứu trong bộ nhớ cho từng SKU thay vì gọi mạng riêng
  từng SKU (bản Python gọi 30-60 lần mạng/đơn — không cache).
- Ghi kết quả ra **file Excel riêng để test** (không phải `dondathang.xlsx`
  thật đang chạy production) — cùng layout cột, cùng quy tắc tô đỏ + comment
  khi giá lệch so với Google Sheets, cùng logic ghi dòng khuyến mãi bám theo
  sản phẩm khi giá khớp.
- Interface `processing.Processor` sửa từ `Process(...) (OrderRow, error)`
  thành `Process(...) ([]OrderRow, error)` để phản ánh đúng thực tế một
  file PDF có thể chứa nhiều đơn hàng. `App.runBatch` (Phase 1) sửa theo:
  duyệt qua từng phần tử trả về, emit `process:row` cho mỗi đơn, tăng STT
  theo **số đơn thật** tìm được (không phải số file).
- Mọi trường hợp bất thường phải **hiện ra** thành một `OrderRow` trạng
  thái Thất bại kèm lý do cụ thể trong `Status` — không được âm thầm bỏ
  qua. Đây là khác biệt có chủ đích so với bản Python (nơi đếm POM lệch
  sẽ khiến trang bị bỏ qua hoàn toàn, không log, không báo).

### Không làm (ngoài phạm vi Phase 2a)

- BigC và mọi vendor khác ngoài Coop — Phase 2b/2c..., spec riêng.
- Upload PDF lên Google Drive qua Google Apps Script — side-effect độc
  lập với việc parse/ghi Excel, để dành làm cùng đợt với Zalo/MISA
  (automation tới dịch vụ ngoài, theo roadmap gốc).
- Ghi trực tiếp vào `dondathang.xlsx` thật ở gốc repo — chỉ chuyển sang
  file thật khi Go app đã sẵn sàng thay thế hẳn `App.py`.
- Port các hàm đã xác nhận là code chết trong `xulydonhang.py`:
  `tachSP_text`, `extract_sku` (hàm rời, không có nơi gọi), `extract_text_from_pdf`
  (logic bị lặp lại inline trong `process_file`), `laymakhachhang_STF`/
  `laymakhachhang_satra` (không có nơi gọi), khối `.xlsx` bị comment gần
  cuối file (không bao giờ chạy).

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go        # Identify(text string) string — nhận Coop qua
                        # "Vendor\s*[-:]\s*(21569|22856)"; trả "" nếu
                        # không khớp (chỗ để mở rộng vendor khác sau)
  coop/
    dispatch.go         # CountPOsOnPage(text) (pomCount, subTotalCount int)
                         # SplitMultiPO(text string) []string
    invoice.go           # ParseInvoiceInfo(text string) (InvoiceInfo, error)
                         # — PO#, location, entry/cancel date, notes, shipto
    extract.go           # ExtractProducts(text string) ([]Product, error)
                         # — heuristic dò khối text giữa các mã SKU
    promo.go              # SplitPromoText(text, system string) string
                         # — quy ước cm/cf
    sku.go                # LoadSkuMapping, ReplaceSkuNumbers, CleanSkuNumber
  productdata/
    store.go              # nạp data.xlsx 1 lần (sheet MaKH, SanPham) →
                         # index trong bộ nhớ; GetCustomerCode,
                         # GetProductInfo, ResolveSystem (COOPMART/COOPFOOD)
  pricing/
    coop.go                # FetchCoopPricing(gid string) (*PricingIndex, error)
                         # — 1 lần fetch CSV/đơn, index theo SKU trong bộ nhớ
                         # PricingIndex là interface — test Go dùng bản
                         # nạp từ fixture đã đóng băng, RealProcessor thật
                         # dùng bản fetch qua HTTP
  excelwriter/
    dondathang.go         # WriteOrderRows(path string, rows []ExcelRow) error
                         # — cột theo layout hiện tại (xem bảng dưới), tô đỏ
                         # + comment khi sai giá, dùng github.com/xuri/excelize/v2
  coop_processor.go        # RealProcessor implement processing.Processor:
                         # điều phối toàn bộ luồng, trả về []OrderRow
  coop_processor_test.go   # đối chiếu với 155 fixture JSON

GO/internal/processing/coop/testdata/
  fixtures/*.json           # sinh bởi script Python, đã đóng băng giá/KM
  generate_fixtures.py       # script throwaway sinh fixture (không phải
                            # một phần của ứng dụng Go)
```

### Layout cột Excel (giữ nguyên từ bản Python, sheet "Don dat hang", header ở dòng 8)

| Cột | Ý nghĩa | Cột | Ý nghĩa |
|---|---|---|---|
| A | Ngày đơn hàng | Q | Mã hàng (SKU nội bộ) |
| B | Số đơn hàng = `ĐĐHCOOP-{PO}` | S | Tên hàng |
| C | Trạng thái (`"Chưa thực hiện"`) | U | Hàng khuyến mại (Có/Không) |
| D | Ngày giao hàng | X | Số lượng |
| E | Địa điểm giao hàng | Y | Đơn giá |
| G | Mã khách hàng | Z | Thành tiền = `Y*X` |
| L | Diễn giải (kèm tổng trọng lượng ở dòng đầu) | AJ | Mã đơn vị (khu vực) |
| AM | Mã thống kê (miền) | AO | Ghi chú (bó kèm/rời khuyến mãi) |
| AP | Mã hàng co (mã ghép khuyến mãi) | AQ | Nội dung khuyến mãi |
| AT | Trọng lượng dòng (kg) | AU | Số kiện (`ceil(qty/pack_size)`) |
| AE | % thuế GTGT (`8`, cố định) | AV | Số ngày được nợ (`60`, cố định) |

Khi giá dòng lệch so với Google Sheets (`math.isclose(rel_tol=1e-4)`): tô
đỏ cột Y + comment `"Kiểm tra lại giá mã này! - Giá hóa đơn: {X} - Chênh
lệch: {Y}"` — giữ nguyên định dạng thông báo từ bản Python.

### Ánh xạ trạng thái OrderRow

Tái dùng `StatusDone/StatusWarning/StatusFailed` đã có từ Phase 1:

- **StatusDone**: parse + ghi Excel thành công, giá khớp Google Sheets.
- **StatusWarning**: parse + ghi Excel thành công nhưng giá lệch (đã tô đỏ
  + comment trong Excel) — tương đương "⚠️Hoàn Thành" của bản cũ.
- **StatusFailed**: bất kỳ lỗi nào ngăn ghi được dòng — đếm POM lệch, không
  nhận diện được vendor, không trích được sản phẩm, SKU không có trong
  `data.xlsx`. `Status` phải chứa lý do cụ thể, không chỉ "Thất bại" trơn.

### Nguồn sự thật cho các heuristic dễ vỡ

`extract_products` (Python: `xulydonhang.py:373-500`), `tachkhuyenmai_coop`
(`:747-784`), và các regex ngày/ghi chú/shipto trong phần đầu
`process_coop_invoice` (`:5362-5450` ước lượng) là logic được tinh chỉnh
thủ công theo cách PDF thật được linearize thành text — không phải một
grammar sạch có thể suy luận lại từ đầu. Kế hoạch triển khai (bước tiếp
theo, viết bằng skill `writing-plans`) phải tham chiếu trực tiếp các dòng
Python này làm nguồn sự thật khi viết từng hàm Go tương ứng, và **mọi hàm
port từ nhóm này bắt buộc có test đối chiếu fixture** trước khi coi là
xong — tài liệu này không liệt kê lại toàn bộ regex vì đó là công việc của
kế hoạch/lúc triển khai, không phải của spec.

## Dữ liệu ngoài & phụ thuộc mạng

- **`data.xlsx`**: đọc 1 lần khi khởi động. Sheet `MaKH` (A=hệ thống,
  B=mã trạm, C=mã KH nội bộ, D=địa chỉ), sheet `SanPham` (A=mã hàng nội
  bộ, B=tên, C=trọng lượng kg, D=quy cách thùng, cột mã hàng riêng từng
  vendor gồm cột mã hàng Coop).
- **`settings.ini`**: khối `<gid>` (không phải INI chuẩn, cần parser tự
  viết như bản Python) — lấy `gid` của Coop để build URL:
  `https://docs.google.com/spreadsheets/d/1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4/gviz/tq?tqx=out:csv&gid=<gid>`.
- **Google Drive upload**: không gọi ở Phase 2a (xem mục Không làm).

## Testing

- Go unit test cho từng package thuần (`vendor`, `coop/dispatch`,
  `coop/invoice`, `coop/extract`, `coop/promo`, `coop/sku`,
  `productdata`, `excelwriter`) theo TDD như Phase 1.
- `coop_processor_test.go`: test tích hợp chạy `RealProcessor` trên toàn
  bộ 155 fixture, dùng `PricingIndex` đã đóng băng (không gọi mạng) — đây
  là lưới an toàn chính, không phải unit test từng hàm nhỏ.
- `productdata`/`excelwriter` dùng bản sao `data.xlsx` trong `testdata/`
  (không đọc file thật ở gốc repo trong lúc test).

## Rủi ro / lưu ý

- `github.com/ledongthuc/pdf` đã xác minh hoạt động tốt trên 1 file Coop
  mẫu; cần xác nhận lại trên diện rộng hơn (qua chính bộ 155 fixture) khi
  triển khai — nếu phát hiện file nào layout khác thường khiến thư viện
  đọc sai, xử lý case đó khi gặp, không chặn toàn bộ tiến độ.
- Google Sheets là dữ liệu sống — fixture "đóng băng" tại thời điểm sinh
  có thể lệch với giá hiện tại nếu sheet đã đổi. Đây là test kiểm tra
  **logic parse + tính toán** đúng theo một bộ dữ liệu giá cố định, không
  phải test "giá hôm nay có đúng không" — nếu cần refresh, chạy lại script
  sinh fixture.
- File PDF có layout bất thường (không đếm được POM/Sub Total khớp nhau)
  sẽ tăng số dòng "Thất bại" so với hành vi im lặng bỏ qua của bản cũ —
  đây là thay đổi hành vi có chủ đích (xem mục Nguyên tắc lỗi), cần người
  dùng biết trước khi thấy nhiều dòng đỏ hơn bản cũ ở những file trước đây
  "biến mất" âm thầm.
