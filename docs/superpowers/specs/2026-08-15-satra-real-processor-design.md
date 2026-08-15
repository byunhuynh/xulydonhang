# Thiết kế: RealProcessor cho Satra (Phase 2b-2 của dự án refactor)

## Bối cảnh

Phase 2b-1 (đã hoàn tất, xem
[2026-08-14-lotte-real-processor-design.md](2026-08-14-lotte-real-processor-design.md))
thêm hỗ trợ Lotte vào `RealProcessor`, đạt 60/60 file mẫu khớp hoàn toàn.
Phase 2b-2 là sub-project tiếp theo trong lộ trình 7 nhà cung cấp, làm
**Satra** — nhà cung cấp có 33 file mẫu thật, nhiều thứ hai sau Lotte.

### Kế thừa từ Phase 2b-1

Cùng chính sách kiểm thử "đúng luồng chính, không bắt buộc giữ bug cũ" đã
thống nhất cho toàn bộ Phase 2b (khác Phase 2a/Coop, vốn bắt buộc giống
100% kể cả bug). Cùng hạ tầng dùng chung: `excelwriter.Row`/`WriteOrderRows`,
`pricing.Index`/`FetchIndex`, và các hàm khuyến mãi/vùng miền đã port ở
Coop/Lotte (`regionInfo`, `closeEnough`, `buildPromoBonusRow`,
`buildInvoiceBonusRow`, `coop.ExtractDiscount`, `coop.FormatWeightKg`,
`productdata.FindSkusMentioned/ResolveSku/GetProductInfo`).

## Dữ liệu đối chiếu (golden corpus)

**33 file PDF Satra thật** tại `đơn hàng/08-2026/*.pdf` (đã có sẵn). Mỗi
file đúng 1 trang, nhận diện qua `identify_vendor`'s Satra pattern
(`VD-00002345` hoặc `VD-00002547` trong text đã chuẩn hóa khoảng trắng).

### Xác minh độ tin cậy (đã làm trước khi viết spec)

Chạy script kiểm tra độc lập trên toàn bộ 33 file thật, kết quả **100%**
cho mọi mốc trích xuất:

| Mốc | Kết quả |
|---|---|
| Số PO (`\*P-[^*]+\*`) | 33/33 |
| Khối ngày đặt hàng (`...\nNgày đặt hàng:`) | 33/33 |
| Khối ngày hủy (`Ngày giao hàng:...Địa chỉ giao hàng:`) | 33/33 |
| Khối sản phẩm (`STT...Hàng phục vụ cho:`) | 33/33 |
| Khối địa chỉ giao hàng (`Địa chỉ giao hàng:...Địa chỉ thanh toán:`) | 33/33 |

**Quan trọng — đã xác minh tra cứu mã khách hàng bằng so khớp mờ hoạt động
đúng trên dữ liệu thật:** gọi trực tiếp hàm Python thật
`laymakhachhang_satra` (không qua GUI) với địa chỉ trích từ cả 33 file,
đối chiếu với 192 dòng hệ thống SATRA trong `data.xlsx` — **kết quả khớp
33/33**, xác nhận ngưỡng `score > 95` hoạt động tốt trên dữ liệu thật hiện
có, không phải giả định lý thuyết.

## Thách thức kỹ thuật mới: so khớp địa chỉ mờ (fuzzy matching)

Khác Coop (khớp chính xác hậu tố) và Lotte (khớp hậu tố đơn giản),
`laymakhachhang_satra` (`xulydonhang.py:263-287`) dùng
`fuzz.partial_ratio` từ thư viện Python `fuzzywuzzy` (`from fuzzywuzzy
import fuzz`, dòng 11) — thuật toán này dựa trên `difflib.SequenceMatcher`
của Python (thuật toán Ratcliff/Obershelp: tìm khối con khớp dài nhất,
trượt cửa sổ so sánh trên chuỗi dài hơn), **không phải Levenshtein
distance** như nhiều thư viện "fuzzy matching" tổng quát khác — cần đúng
thuật toán này để cho kết quả khớp Python, không thể dùng bừa một thư
viện fuzzy matching Go bất kỳ.

**Hai lựa chọn đã khảo sát (cả hai đều `go get` được, đã xác nhận qua
`go list -m`):**
1. `github.com/paul-mannino/go-fuzzywuzzy` — bản port trực tiếp từ chính
   `fuzzywuzzy`, khả năng cao khớp thuật toán chính xác nhất, nhưng không
   có phiên bản gắn thẻ (semver), khó xác định mức độ bảo trì/đúng đắn chỉ
   qua tên.
2. `github.com/pmezard/go-difflib` (đã có bản v1.0.0 ổn định) — bản port
   `difflib`/`SequenceMatcher` được dùng rộng rãi, đáng tin cậy hơn về
   chất lượng, nhưng cần tự viết hàm `partial_ratio` bên trên (dựa theo
   đúng thuật toán `fuzzywuzzy`'s `partial_ratio` — tìm khối khớp dài nhất
   qua `SequenceMatcher.get_matching_blocks()`, thử các cửa sổ căn theo
   khối đó trên chuỗi dài hơn, lấy `ratio()` cao nhất).

**Quyết định cụ thể để dành cho lúc viết plan**, dựa trên đối chiếu
golden-fixture: thử `go-fuzzywuzzy` trước; nếu kết quả khớp không đạt
33/33 khi đối chiếu với fixture, chuyển sang tự viết `partial_ratio` trên
`go-difflib` — **bắt buộc đối chiếu bằng golden-fixture thật, không suy
luận lý thuyết**, đúng nguyên tắc đã áp dụng xuyên suốt dự án.

`normalize_text` (`xulydonhang.py:217-222`, dùng chung với các vendor
khác) — chuyển thường, xóa ký tự đặc biệt, chuẩn hóa khoảng trắng — port
đơn giản, không có thách thức đặc biệt.

## Phạm vi

### Làm thật

- Trích xuất Satra đầy đủ, mỗi trang PDF độc lập (mỗi file mẫu hiện có
  đúng 1 trang, không có bằng chứng multi-page nhưng logic Python không
  loại trừ — port theo đúng cấu trúc per-page như Coop/Lotte):
  - Số PO: `*P-...*` giữa 2 dấu `*`, bỏ 2 ký tự đầu/cuối
    (`xulydonhang.py:9309-9310`).
  - Ngày đặt hàng: dòng cuối cùng trước `"Ngày đặt hàng:"`, định dạng
    `%m/%d/%Y` → `%d/%m/%Y`; **có fallback**: nếu kết quả là
    `01/01/0001` (ngày rỗng/không hợp lệ), thử lại với mốc `"Ngày in:"`
    thay vì `"Ngày đặt hàng:"` (`:9326-9336`).
  - Ngày hủy: dòng đầu tiên có định dạng ngày giữa `"Ngày giao hàng:"` và
    `"Địa chỉ giao hàng:"`, cùng định dạng `%m/%d/%Y` → `%d/%m/%Y`
    (`:9339-9347`).
  - Địa chỉ giao hàng: khối text giữa `"Địa chỉ giao hàng:"` và `"Địa chỉ
    thanh toán:"`, ghép nhiều dòng thành 1, chuẩn hóa khoảng trắng kép
    (`:9312-9314`).
  - Danh sách sản phẩm: cắt tại `"Tổng cộng"`, tách khối theo mã vạch
    13 chữ số đứng đầu dòng sau số thứ tự, tìm dòng có dạng `"N,000"` làm
    số lượng, dòng kế tiếp làm thành tiền, bỏ qua nếu giá = 0
    (`trichxuatsanpham_satra`, `:6492-6529`).
  - Mã khách hàng: so khớp mờ địa chỉ giao hàng với cột D (địa chỉ) của
    `data.xlsx` sheet `MaKH`, lọc theo cột A chứa `"SATRA"` (khớp
    **substring**, không phải khớp chính xác — `col_A.upper() in "SATRA"`
    trong Python, không phải `col_A.upper() == "SATRA"` — port đúng ngữ
    nghĩa này), trả về cột C của dòng khớp cao nhất nếu điểm > 95
    (`laymakhachhang_satra`, `:263-287`).
  - Tính giá/khuyến mãi thực tế, dòng khuyến mãi tặng kèm: tái dùng
    nguyên vẹn logic đã port (xác nhận cấu trúc `write_to_dondathang_satra`
    giống hệt Lotte — cùng vòng lặp `find_price_by_sku`/
    `find_all_promotions_by_sku_and_time`/`extract_discount`, cùng công
    thức `math.isclose(rel_tol=1e-4)`, cùng quy tắc tô đỏ + comment).
  - Vùng miền/kho (`kho`/`khuvuc`/`mien`): **giống hệt logic đã có** ở
    `regionInfo` (rẽ nhánh theo `makhachhang[:2] == "MB"`) — tái dùng trực
    tiếp, không viết lại.
  - Ghi chú "Không giao thứ 7": khi `makhachhang == "MN_MT_stph"`, diễn
    giải (cột L) có thêm hậu tố `"- Không giao thứ 7"` (`:2370-2374`) —
    chi tiết nhỏ, cần port đúng.
- Ghi kết quả ra file Excel test riêng, tái dùng nguyên `excelwriter.Row`/
  `WriteOrderRows` — đã xác nhận layout cột giống 100% Coop/Lotte.
- Mở rộng `vendor.Identify` nhận diện Satra.
- Mở rộng dispatch của `RealProcessor.Process` sang nhánh Satra (theo
  đúng mẫu switch đã có từ Phase 2b-1's file `lotte_processor.go` —
  Satra sẽ có `satra_processor.go` riêng ngay từ đầu, không lặp lại việc
  phải tách file sau như Lotte đã phải làm ở review cuối).
- Thêm hàm tra cứu mã khách hàng kiểu Satra vào `productdata.Store` (so
  khớp mờ + lọc hệ thống theo substring) — hàm mới, không sửa
  `GetCustomerCode`/`GetCustomerCodeBySuffix` hiện có.

### Không làm (ngoài phạm vi Phase 2b-2)

- Winmart, BigC, Emart, FujiMart, Kingfood — sub-project riêng sau.
- 2 nơi khác trong `xulydonhang.py` cũng gọi `write_to_dondathang_satra`
  (`:9647-9714` và `:10204-10213`) — xác nhận đây là **luồng nhập liệu
  khác** (dữ liệu đã gộp nhóm theo `po_number`/`he_thong` từ một nguồn
  không phải PDF trực tiếp qua vòng lặp trang — có thể là luồng Excel/API
  tổng hợp nhiều vendor). Không thuộc phạm vi "xử lý PDF thật" của phase
  này, giống cách các luồng phụ tương tự đã bị loại ở Coop/Lotte.
- Upload PDF lên Google Drive, ghi trực tiếp vào `dondathang.xlsx` thật —
  cùng lý do đã áp dụng từ Phase 2a.

## Kiến trúc

```
GO/internal/processing/
  vendor/
    identify.go          # + pattern nhận diện Satra:
                          # "VD-00002345" hoặc "VD-00002547"
  satra/
    extract.go            # ParsePONumber(text) string
                          # ParseEntryDate(text) (string, error) — có
                          # fallback "Ngày in:" khi kết quả rỗng
                          # ParseCancelDate(text) (string, error)
                          # ParseShipToAddress(text) string
                          # ExtractProducts(text) ([]Product, error)
    fuzzymatch.go          # PartialRatio(a, b string) int — thuật toán
                          # fuzzywuzzy's partial_ratio, dùng thư viện Go
                          # tương đương (xem mục "Thách thức kỹ thuật")
  productdata/
    store.go               # + GetCustomerCodeByFuzzyAddress(system,
                          # address string) (code string, ok bool)
  satra_processor.go        # processSatraSegment — mirror
                          # processLotteSegment's cấu trúc, tái dùng
                          # regionInfo/closeEnough/buildPromoBonusRow/
                          # buildInvoiceBonusRow
  satra_processor_test.go
  satra_golden_test.go      # đối chiếu 33 fixture JSON, tái dùng
                          # compareRowsAgainstFixture/fixtureData/
                          # fixturePricingSource đã có

GO/internal/processing/satra/testdata/
  fixtures/*.json            # sinh bởi script Python throwaway
  generate_fixtures.py
```

### Bài học kiến trúc từ Phase 2b-1 áp dụng ngay từ đầu

Review toàn nhánh của Lotte (Phase 2b-1) phát hiện việc gộp code xử lý
Lotte vào file `coop_processor.go` (dùng chung với Coop) đã gây khó bảo
trì và trực tiếp góp phần vào 1 lỗi thật (nhầm copy logic tách chuỗi `"|"`
của Coop sang Lotte). Satra sẽ có file `satra_processor.go` **riêng ngay
từ Task đầu tiên** viết nó, không đợi đến review cuối mới tách.

### Ánh xạ trạng thái OrderRow

Tái dùng nguyên `StatusDone/StatusWarning/StatusFailed`.

## Dữ liệu ngoài & phụ thuộc mạng

- **`data.xlsx`**: sheet `MaKH` — thêm truy vấn so khớp mờ theo cột D,
  lọc theo cột A chứa (substring) `"SATRA"`. Test fixture
  (`productdata/testdata/data.xlsx`) cần thêm ít nhất 1 dòng hệ thống
  SATRA với địa chỉ mẫu để viết test cho hàm mới — bổ sung ở task tương
  ứng, không sửa dữ liệu đã có.
- **`settings.ini`**: `<gid>` đã có sẵn `SATRA = 2037775257`.
- **Google Drive upload**: không gọi ở Phase 2b-2.

## Testing

- Go unit test cho từng hàm trích xuất thuần trong `satra/` (TDD).
- Test riêng cho `PartialRatio`/hàm so khớp mờ, đối chiếu với kết quả
  Python thật trên các cặp chuỗi mẫu lấy từ dữ liệu thật (đã có sẵn từ
  bước xác minh của spec này — 33 cặp địa chỉ/kết quả khớp).
- `satra_golden_test.go`: đối chiếu toàn bộ 33 fixture, dùng
  `PricingSource` đã đóng băng — lưới an toàn chính. Áp dụng chính sách
  ngoại lệ có tài liệu (`knownDivergences_Satra`, key theo
  `sourcePDF:row:col` — đã sửa đúng dạng khóa này ở Phase 2b-1's review
  cuối, dùng lại ngay từ đầu cho Satra).

## Rủi ro / lưu ý

- **So khớp mờ là rủi ro kỹ thuật lớn nhất của phase này** — thư viện Go
  chọn được phải cho kết quả khớp giống Python trên toàn bộ 33 file thật,
  không chỉ khớp về mặt khái niệm "fuzzy matching nói chung". Nếu cả 2
  lựa chọn thư viện đều không đạt độ khớp cần thiết, phương án dự phòng
  là tự triển khai đầy đủ thuật toán `partial_ratio` bằng tay (không
  dùng thư viện ngoài) — quyết định cụ thể để dành lúc viết plan.
- Đã xác minh 33/33 file mẫu hoạt động đúng với hàm Python thật — nhưng
  đây vẫn chỉ là 33 mẫu; nếu tương lai có thêm file Satra không khớp
  ngưỡng >95%, hành vi kỳ vọng là dòng "Thất bại" rõ ràng (giống nguyên
  tắc lỗi đã lập từ Phase 2a), không phải một giá trị đoán mò.
- Cột A của `data.xlsx`'s sheet `MaKH` được lọc bằng phép `in` (chứa
  chuỗi con) chứ không phải so sánh bằng — cần port đúng, không được đơn
  giản hóa thành so sánh bằng khi viết Go.
