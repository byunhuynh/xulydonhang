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
hiện tại: mỗi trang store phụ thuộc dữ liệu đã trích từ trang 0 (bảng
giá, mã khách hàng, PO/ngày) — trạng thái xuyên trang thật sự, không thể
xử lý từng trang độc lập.

**Đính chính sau khi đọc kỹ (lúc viết plan):** lý do ban đầu ghi ở đây
("dấu hiệu nhận diện BigC chỉ xuất hiện ở trang 0") **sai** — pattern
nhận diện thật (`xulydonhang.py:99`) là
`"3005382" in text or "CTY TNHH DV EB" in text` (không phân biệt hoa
thường), và `"CTY TNHH DV EB"` thực ra xuất hiện ở **mọi trang**, kể cả
trang store (xác nhận trên cả 6 file mẫu kiểm tra). Vậy `vendor.Identify`
per-page vẫn sẽ nhận đúng BigC ở mọi trang nếu xét riêng — **đây không
phải lý do kiến trúc cần pre-check + handler riêng**. Lý do thật sự (và
vẫn đứng vững) là **trạng thái xuyên trang**: trang store cần dữ liệu đã
trích từ trang 0 (bảng giá theo barcode, mã khách hàng, PO/ngày) mà một
lệnh gọi `Identify`+dispatch độc lập trên riêng trang đó không thể tự có
được.

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

**Đã xác nhận khi viết plan: Python thật KHÔNG cô lập lỗi theo trang —
đây là quyết định chủ động mới bên Go, đã được xác nhận rõ ràng, không
phải port hành vi cũ.** `process_file`'s vòng lặp `for page_num in
range(len(doc))` (`:7210`) không có try/except nào bao quanh (khối
try/except cấp file từng có đã bị comment-out ở `:7196`/`:10302-10304`).
Lỗi duy nhất được bắt là ở `App.py`'s `ProcessThread.run()` (`:80-87`),
bọc quanh **toàn bộ** `process_file(file, stt)` — tức 1 trang lỗi
(exception nào cũng được, kể cả `TypeError` khi `entry_date=None` lọt
vào `convert_entry_date`) sẽ làm crash và **bỏ qua hoàn toàn** mọi trang
sau đó trong cùng file (không phải "cô lập", mà là "mất luôn"), chỉ các
trang trước đó (đã `wb.save()` xong) mới giữ được kết quả.

Go sẽ **chủ động cải thiện** hành vi này (đúng chính sách "đúng luồng
chính, không cần giữ bug cũ" của Phase 2b):

- **Trang 0 lỗi** (không trích được PO/ngày/bảng giá) → **fail cả file**
  (1 `OrderRow` Failed duy nhất) — vì mọi trang store đều phụ thuộc dữ
  liệu trang 0, không thể xử lý tiếp một cách có ý nghĩa.
- **Một trang store lỗi** (danh sách hàng dị dạng, không ghép được giá,
  ...) → **cô lập lỗi theo từng trang** — 1 `OrderRow` Failed riêng cho
  trang đó, các trang store khác (kể cả các trang SAU trang lỗi) vẫn xử
  lý bình thường. Đây là điểm khác biệt thật sự so với Python, đã xác
  nhận và quyết định chủ động.

## Mã khách hàng: bảng tra cứu cứng, KHÔNG so khớp mờ

Khác Satra (fuzzy match `data.xlsx`), BigC dùng logic if/elif cứng dựa
trên text trang 0, **không tra `data.xlsx`, không fuzzy match**. **Đã
đính chính khi viết plan — bảng thật là phép chéo (cross) giữa 2 mã nhà
cung cấp × 2 tên kho, kiểm tra bằng `in` (substring) trên toàn bộ text
trang 0** (`xulydonhang.py:9419-9433`, biến `trangdaubigc = text`):

```python
if "3006900" in trangdaubigc and "LINFOX WAREHOUSE (802)" in trangdaubigc:
    makhachhang = "MB_GC_BIGC";   diachigiao = "LINFOX WAREHOUSE (802)"
elif "3005382" in trangdaubigc and "LINFOX WAREHOUSE (802)" in trangdaubigc:
    makhachhang = "MB_MT_BIGC";   diachigiao = "LINFOX WAREHOUSE (802)"
elif "3005382" in trangdaubigc and "FM LOGISTIC VSIP 2 (806)" in trangdaubigc:
    makhachhang = "MN_MT_BIGCAC"; diachigiao = "FM LOGISTIC VSIP 2 (806)"
elif "3006900" in trangdaubigc and "FM LOGISTIC VSIP 2 (806)" in trangdaubigc:
    makhachhang = "MN_GC_BIGCAC"; diachigiao = "FM LOGISTIC VSIP 2 (806)"
else:
    makhachhang = "MN_MT_BIGCAC"; diachigiao = "FM LOGISTIC VSIP 2 (806)"
```

**Quan trọng:** hàm trả về **2 giá trị** — `makhachhang` (mã khách hàng)
VÀ `diachigiao` (địa chỉ kho giao hàng, dùng cho cột `E` khi ghi Excel).
Port thành 1 hàm thuần Go `bigc.ResolveCustomerCode(pageZeroText string) (customerCode, deliveryWarehouse string)`
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
  `regionInfo`/`closeEnough` (đã parameterize orderNumber ở Task 1)
  nhưng **KHÔNG tái dùng thẳng** `buildPromoBonusRow`/
  `buildInvoiceBonusRow` — **đã xác nhận khi viết plan (đọc kỹ
  `write_to_dondathang_bigc`, `:4541-4897`): cấu trúc khác thật sự với
  Coop/Satra**, cần bộ dựng row riêng kiểu Lotte:
  - **Không có** vòng lặp tách `khuyenmai.split('|')` + `enumerate` —
    chỉ xử lý 1 chuỗi khuyến mãi trực tiếp mỗi item.
  - Text mặc định khi không "che barcode" là `"KM Rời - Không Che Barcode"`
    (khác `"KM Bó Kèm - Che Barcode"` của Coop/Satra) cho nhánh per-item;
    riêng nhánh khuyến mãi cấp đơn hàng (`kmhoadon`) thì dùng lại đúng
    `"KM Bó Kèm - Che Barcode"` như Coop/Satra.
  - **Không hề ghi cột `AU`** (số kiện hàng) ở bất kỳ đâu trong hàm — Go
    KHÔNG được port hành vi ghi AU của `buildInvoiceBonusRow` vào bộ
    dựng row của BigC.
  - Dòng header/tổng chỉ ghi **1 lần duy nhất cho cả file** (khi
    `page_num == 1`, tức trang store đầu tiên) — các trang store sau chỉ
    nối thêm row sản phẩm, không có row header riêng. Tổng khối lượng
    (`bat_dau`) chỉ tính ở trang cuối, cộng dồn từ `start_row` (ghi nhận
    lúc trang 0) đến `current_row` hiện tại — ghi đè lại vào ô `L` của
    dòng header (đã ghi từ trang 1).
  - Giá ghi cột `Y` khi khớp là `giathucte` (giá hệ thống tính) — **giống
    Coop, khác Satra** (Satra ghi giá hóa đơn).
  - Không có nhánh đặc biệt kiểu "Không giao thứ 7" của Satra.
  - `kho`/`khuvuc`/`mien`: `MB_GC_BIGC` và `MB_MT_BIGC` **dùng chung** 1
    nhánh (`makhachhang[:2] == "MB"`) — không phân biệt GC/MT ở tầng
    vùng miền dù mã khách hàng có encode khác nhau. Không có nhánh `else`
    trong Python (sẽ crash `UnboundLocalError` nếu lọt) — Go cần có
    default tường minh (báo lỗi rõ ràng, không panic ngầm) vì
    `ResolveCustomerCode` chỉ trả về 4 giá trị cố định nên về lý thuyết
    không bao giờ lọt vào default, nhưng vẫn nên viết phòng thủ.
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
                              # ResolveCustomerCode (trả về 2 giá trị:
                              # customerCode, deliveryWarehouse),
                              # ExtractStoreName, ExtractStoreItems,
                              # JoinItemsWithPrices
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
  Trang cuối được xử lý y hệt trang giữa (không port bước upload Drive) —
  **đã xác nhận thêm 1 khác biệt thật sự ở trang cuối**: tổng khối lượng
  (`bat_dau`) chỉ được tính và ghi đè vào dòng header ở trang cuối cùng
  (xem mục "Kiến trúc" → `bigc_processor.go`), nên trang cuối KHÔNG hoàn
  toàn giống trang giữa như spec bản đầu giả định — cần port đúng bước
  "tổng kết cuối file" này.
- `laydanhsachsanpham_bigc`'s dict key `"Total Price"` thực ra chứa
  **đơn giá** (`net_price`, đã trim `","`), không phải tổng tiền dòng —
  tên khóa gây hiểu nhầm nhưng phải port đúng tên/ý nghĩa này khi viết
  Go struct (đặt tên field Go rõ ràng hơn, ví dụ `UnitPrice`, kèm comment
  trỏ về tên khóa Python gốc).
- `ghepgia_donhangbigc`: nếu barcode của 1 item trong trang store không
  có trong bảng giá trang 0, Python **âm thầm gán giá = 0** (comment
  Python nói "báo lỗi" nhưng code thật không báo gì cả) — port đúng hành
  vi im lặng này, không tự ý thêm cảnh báo/lỗi mới ở bước ghép giá (có
  thể xử lý ở bước sau, ví dụ đánh dấu sai giá, tùy vào cách
  `closeEnough`/so khớp giá hiện có phản ứng với giá 0).
- 29 file mẫu hiện có — nếu tương lai có thêm file BigC, hành vi kỳ vọng
  vẫn là dòng "Thất bại" rõ ràng khi không parse được, không phải giá trị
  đoán mò.
