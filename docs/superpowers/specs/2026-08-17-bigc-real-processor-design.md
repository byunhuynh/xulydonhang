# Thiết kế: RealProcessor cho BigC (Phase 2b-3 của dự án refactor)

## Bối cảnh

Phase 2b-1 (Lotte, 60/60) và Phase 2b-2 (Satra, 36/36) đã hoàn tất và
đóng. Phase 2b-3 là sub-project tiếp theo trong lộ trình 7 nhà cung cấp,
làm **BigC** — nhà cung cấp có 29 file mẫu thật (tăng từ ước tính 27 lúc
lập lộ trình ban đầu, do có thêm file mới).

### Kế thừa từ Phase 2b-1/2b-2

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ" đã
thống nhất cho toàn bộ Phase 2b (khác Phase 2a/Coop). Cùng hạ tầng dùng
chung: `excelwriter.Row`/`WriteOrderRows`, `pricing.Index`/`FetchIndex`.

### Nợ kiến trúc mang từ review cuối của Satra — gộp vào Task 0/1 của phase này

Review toàn nhánh của Satra (Phase 2b-2) phát hiện 3 việc cần làm "trước
khi làm vendor #4" — quyết định của phase này là **gộp cả 3 vào làm sớm**,
không để lùi thêm lần nữa:

1. **Tách lớp helper vendor-trung lập** — `regionInfo`, `closeEnough`,
   `buildPromoBonusRow`, `buildInvoiceBonusRow`, `coopDebtDays`,
   `stripBlankLines` hiện vẫn nằm rải rác trong `coop_processor.go`/
   `lotte_processor.go` dù đã được 3 vendor dùng chung — chuyển sang file
   mới `processor_shared.go`. Tương tự các helper test dùng chung
   (`fixtureData`, `compareRowsAgainstFixture`, `joinLines`,
   `copyTestWorkbookForProcessor`) chuyển sang `golden_test_helpers_test.go`.
   Refactor thuần túy — không đổi hành vi, mọi test Coop/Lotte/Satra hiện
   có phải xanh y hệt trước/sau.
2. **Parameterize order-number của `buildPromoBonusRow`/
   `buildInvoiceBonusRow`** — 2 hàm này hiện hardcode
   `orderNumber()` kiểu Coop bên trong, buộc Lotte và Satra phải
   post-patch `bonusRow.OrderNumber = ...` sau mỗi lần gọi (không có lưới
   an toàn compiler/test nếu quên patch). Thêm tham số `orderNumber string`
   vào cả 2 hàm, xóa hết post-patch ở `lotte_processor.go` và
   `satra_processor.go`. BigC dùng ngay dạng có tham số này, không tạo
   thêm bản post-patch thứ 3.
3. **Sửa thứ tự case trong `vendor.Identify`** — thứ tự thật của Python
   (`identify_vendor`, `xulydonhang.py:90-179`) là **Coop → BigC → Lotte
   → Satra → ...** (BigC được kiểm tra ngay sau Coop, trước Lotte). Go
   hiện chỉ có Coop → Lotte → Satra (đúng thứ tự tương đối chỉ vì BigC
   chưa tồn tại). Việc này **bắt buộc** phải làm đúng khi thêm BigC —
   chèn case BigC giữa Coop và Lotte, không phải nối vào cuối.

## Dữ liệu đối chiếu (golden corpus)

**29 file PDF BigC thật** tại `đơn hàng/08-2026/*.pdf`, nhận diện qua
`identify_vendor`'s BigC pattern (`xulydonhang.py:99`):
`re.search(r"3005382", text) or re.search(r"CTY TNHH DV EB", text, re.IGNORECASE)`
— chuỗi này chỉ xuất hiện ở **trang 0** của mỗi file (trang bìa/thông tin
đơn hàng), không xuất hiện lại ở các trang sau.

Ví dụ file: `2631057733376.pdf`, `2631057774005.pdf`, `2631057837378.pdf`,
`2632057860744.pdf`, `2632058001889.pdf`, `2633058028372.pdf`.

## Thách thức kỹ thuật mới: luồng xử lý theo vị trí trang (không phải 1 trang = 1 đơn)

Khác hẳn Coop/Lotte/Satra (mỗi trang/segment là 1 đơn độc lập, tự chứa đủ
thông tin), BigC có cấu trúc **dàn trải theo vị trí trang trong 1 file**,
đã được ghi nhận trước đó khi quyết định KHÔNG gộp BigC vào Phase 2a
(`docs/superpowers/specs/2026-08-14-coop-real-processor-design.md:16-24`):

- **Trang 0**: bảng giá/sản phẩm gốc toàn đơn hàng (`laydanhsachsanpham_bigc`,
  `xulydonhang.py:5831-5873`) + số PO/ngày đặt hàng
  (`trichxuatinfo_donbigc`, `:5941-5973`) + mã khách hàng (logic if/elif
  cứng, `:9419-9433`). **Không ghi Excel row cho trang này.**
- **Trang 1..N-1 (mỗi trang = 1 store)**: tên store (`lay_ten_store`,
  `:5878-5884`) + danh sách hàng của store đó, KHÔNG có giá
  (`trichxuatdanhsachforstore_bigc`, `:5900-5907`) — ghép giá từ bảng
  trang 0 theo barcode (`ghepgia_donhangbigc`, `:5888-5897`) — rồi ghi 1
  bộ Excel row cho store này. Trang cuối (N-1) trong Python còn kích hoạt
  upload PDF lên Google Drive — **Go không port bước này** (đã xác nhận
  Go's `RealProcessor` hiện không có logic Drive upload ở bất kỳ vendor
  nào, kể cả Coop/Lotte/Satra), nên trang cuối được xử lý **y hệt** các
  trang store khác, không cần nhánh riêng.

Điều này phá vỡ giả định của `RealProcessor.Process`'s vòng lặp per-page
hiện tại: `vendor.Identify(text)` chạy độc lập trên từng trang, nhưng chỉ
trang 0 mang dấu hiệu nhận diện BigC — các trang store (1..N-1) sẽ không
khớp `Identify` nếu xét riêng từng trang.

### Quyết định kiến trúc: pre-check toàn file + handler riêng (không đổi luồng per-page hiện có)

Đã cân nhắc 2 phương án, chọn phương án ít xâm lấn nhất:

**Đã chọn — Pre-check + `processBigcDocument`:** Trong `Process()`, thêm
bước kiểm tra `pageTexts[0]` với pattern BigC **trước** vòng lặp per-page
hiện tại. Nếu khớp, bỏ qua vòng lặp per-page cho file này, gọi thẳng
`p.processBigcDocument(pageTexts, filePath, stt) ([]OrderRow, error)` xử
lý toàn bộ file cùng lúc (có trạng thái xuyên trang: bảng giá trang 0 →
dùng cho mọi trang store). Coop/Lotte/Satra's per-page dispatch **không
đổi gì**.

**Đã loại — Refactor `Process`/`vendor.Identify` sang dispatch toàn văn
bản (whole-document) cho mọi vendor:** kiến trúc dài hạn "sạch" hơn (khớp
sát cấu trúc Python's `process_file` hơn), nhưng đụng vào code đã ship,
đã test kỹ của Coop/Lotte/Satra mà không mang lợi ích chức năng gì cho 3
vendor đó — rủi ro/công sức không tương xứng với lợi ích trong phase này.

### Xử lý lỗi

- **Trang 0 lỗi** (không trích được PO/ngày/bảng giá) → **fail cả file**
  (1 `OrderRow` Failed duy nhất) — vì mọi trang store đều phụ thuộc dữ
  liệu trang 0, không thể xử lý tiếp một cách có ý nghĩa.
- **Một trang store lỗi** (danh sách hàng dị dạng, không ghép được giá,
  ...) → **cô lập lỗi theo từng trang** — 1 `OrderRow` Failed riêng cho
  trang đó, các trang store khác trong cùng file vẫn xử lý bình thường.
  Nhất quán với cách Coop hiện cô lập lỗi theo từng segment/order. **Cần
  xác nhận lại hành vi thật của Python** (`process_file`,
  `xulydonhang.py:9404-9536`) khi viết plan — thiết kế này là quyết định
  chủ động theo policy "đúng luồng chính" của Phase 2b, không nhất thiết
  phải giống Python 100% nếu Python thật sự fail cả file khi 1 trang lỗi.

## Mã khách hàng: bảng tra cứu cứng, KHÔNG so khớp mờ

Khác Satra (fuzzy match `data.xlsx`), BigC dùng logic if/elif cứng dựa
trên 2 tín hiệu ở trang 0, **không tra `data.xlsx`, không fuzzy match**
(`xulydonhang.py:9419-9433`):

| Tín hiệu 1 (chuỗi) | Tín hiệu 2 (kho) | Mã khách hàng |
|---|---|---|
| `"3006900"` | `"LINFOX WAREHOUSE (802)"` | `MB_GC_BIGC` |
| `"3006900"` | khác | `MB_MT_BIGC` |
| `"3005382"` | `"FM LOGISTIC VSIP 2 (806)"` | `MN_GC_BIGCAC` |
| `"3005382"` | khác | `MN_MT_BIGCAC` |
| (không khớp cả 2) | — | `MN_MT_BIGCAC` (mặc định) |

Port thành 1 hàm thuần Go `bigc.ResolveCustomerCode(pageZeroText string) string`
trong package `bigc` — không đưa vào `productdata.Store` vì không truy
vấn `data.xlsx`.

## Phạm vi

### Làm thật

- **Task 0**: Tách helper vendor-trung lập → `processor_shared.go` +
  `golden_test_helpers_test.go` (mô tả ở mục "Nợ kiến trúc" trên).
- **Task 1**: Parameterize `orderNumber` cho `buildPromoBonusRow`/
  `buildInvoiceBonusRow`, xóa post-patch ở Lotte/Satra.
- **Task 2**: Sửa `vendor.Identify` — chèn case BigC giữa Coop và Lotte,
  pattern `"3005382"` hoặc `"CTY TNHH DV EB"` (không phân biệt hoa
  thường).
- Package mới `GO/internal/processing/bigc/`:
  - Trích PO/ngày đặt hàng từ trang 0 (`ParsePOAndEntryDate`, mirror
    `trichxuatinfo_donbigc`, `:5941-5973`).
  - Trích ngày hủy từ trang 0 (sau lần cuối "Total Net Purchase Price",
    fallback `entry_date + 5 ngày` nếu không tìm thấy).
  - Trích bảng giá/sản phẩm gốc trang 0 (`ExtractPriceList`, mirror
    `laydanhsachsanpham_bigc`, `:5831-5873`).
  - `ResolveCustomerCode` (mô tả ở mục trên).
  - Trích tên store từ 1 trang store (`ExtractStoreName`, mirror
    `lay_ten_store`, `:5878-5884`).
  - Trích danh sách hàng của 1 trang store, không giá
    (`ExtractStoreItems`, mirror `trichxuatdanhsachforstore_bigc`,
    `:5900-5907`).
  - Ghép giá từ bảng trang 0 vào danh sách hàng của từng store theo
    barcode (`JoinItemsWithPrices`, mirror `ghepgia_donhangbigc`,
    `:5888-5897`).
- `bigc_processor.go` (mới) — `processBigcDocument`, tái dùng
  `regionInfo`/`closeEnough`/`buildPromoBonusRow`/`buildInvoiceBonusRow`
  (đã parameterize orderNumber ở Task 1) — cần xác nhận lại khi viết plan
  liệu logic khuyến mãi/bonus-row của `write_to_dondathang_bigc`
  (`:4541-4897`) có "giống hệt cấu trúc Coop/Satra" như quan sát ban đầu
  hay có khác biệt cần override riêng (kiểu Lotte).
- `bigc_processor_test.go`, `bigc_golden_test.go`.
- Mở rộng dispatch của `Process` — pre-check + gọi `processBigcDocument`
  (mô tả ở mục "Quyết định kiến trúc" trên).

### Không làm (ngoài phạm vi Phase 2b-3)

- Winmart, Emart, FujiMart, Kingfood — sub-project riêng sau.
- Upload PDF lên Google Drive — Go không port bước này ở bất kỳ vendor
  nào (đã xác nhận), không phải riêng BigC.
- Ghi trực tiếp vào `dondathang.xlsx` thật ngoài quy trình test.

## Kiến trúc

```
GO/internal/processing/
  processor_shared.go        # MỚI (Task 0) — regionInfo, closeEnough,
                              # buildPromoBonusRow, buildInvoiceBonusRow
                              # (parameterized orderNumber — Task 1),
                              # coopDebtDays, stripBlankLines
  golden_test_helpers_test.go # MỚI (Task 0) — fixtureData,
                              # compareRowsAgainstFixture, joinLines,
                              # copyTestWorkbookForProcessor
  vendor/
    identify.go               # + case BigC, CHÈN giữa Coop và Lotte
                              # (Task 2)
  bigc/
    extract.go                 # ParsePOAndEntryDate, ExtractPriceList,
                              # ResolveCustomerCode, ExtractStoreName,
                              # ExtractStoreItems, JoinItemsWithPrices
  bigc_processor.go            # processBigcDocument — nhận toàn bộ
                              # []string (pageTexts), trả về []OrderRow
  bigc_processor_test.go
  bigc_golden_test.go           # đối chiếu 29 fixture JSON

GO/internal/processing/bigc/testdata/
  fixtures/*.json
  generate_fixtures.py
```

### Ánh xạ trạng thái OrderRow

Tái dùng nguyên `StatusDone/StatusWarning/StatusFailed`.

## Dữ liệu ngoài & phụ thuộc mạng

- **`data.xlsx`**: BigC KHÔNG truy vấn — không cần sửa gì.
- **`settings.ini`**: cần xác nhận `<gid>` đã có sẵn entry cho BigC khi
  viết plan (theo ghi nhận trước đây, khối `<gid>` đã có entry cho toàn
  bộ 7+ vendor).
- **Google Drive upload**: không gọi ở phase này (như mọi phase trước).

## Testing

- Go unit test cho từng hàm trích xuất thuần trong `bigc/` (TDD).
- `bigc_processor_test.go`: test riêng cho luồng pre-check + dispatch
  toàn file, và cho từng nhánh xử lý lỗi (trang 0 lỗi vs. 1 trang store
  lỗi).
- Sau Task 0 (tách file): chạy lại toàn bộ test suite Coop/Lotte/Satra
  hiện có, xác nhận xanh y hệt trước/sau — không cần test mới cho việc
  tách file này, chỉ cần không đổi hành vi.
- `bigc_golden_test.go`: đối chiếu toàn bộ 29 fixture (script Python
  throwaway mirror cấu trúc `generate_fixtures.py` đã có, không cần đổi
  shape JSON vì 1 file đã có thể sinh nhiều row từ trước — kiểu Coop's
  multi-segment). Áp dụng `knownDivergences_BigC` theo đúng key format
  `sourcePDF:row:col`.

## Rủi ro / lưu ý

- **Rủi ro kiến trúc lớn nhất**: đây là vendor đầu tiên cần trạng thái
  xuyên trang (trang 0 → mọi trang store) — khác hẳn mọi vendor trước.
  Nếu về sau phát hiện các trang store cũng cần biết trang có phải trang
  cuối hay không (để làm gì đó khác upload Drive), thiết kế hiện tại (xử
  lý trang cuối y hệt trang giữa) sẽ cần xem lại — nhưng hiện chưa có
  bằng chứng nào cho việc này ngoài upload Drive.
- **Chưa xác nhận cấu trúc chính xác của `write_to_dondathang_bigc`**
  (`:4541-4897`, khá dài — 356 dòng) so với Coop/Satra — quan sát ban đầu
  là "giống cấu trúc", nhưng cần đọc kỹ khi viết plan trước khi khẳng
  định tái dùng `buildPromoBonusRow`/`buildInvoiceBonusRow` nguyên vẹn
  không cần override (Lotte đã từng cần override, Satra thì không).
- **Chưa xác nhận hành vi lỗi thật của Python** khi 1 trang store lỗi
  giữa chừng — thiết kế "cô lập lỗi theo từng trang" là quyết định chủ
  động theo policy Phase 2b, cần đối chiếu lại khi viết plan/golden test.
- 29 file mẫu hiện có — nếu tương lai có thêm file BigC, hành vi kỳ vọng
  vẫn là dòng "Thất bại" rõ ràng khi không parse được, không phải giá trị
  đoán mò.
