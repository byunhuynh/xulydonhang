# Nhánh xử lý đơn TMĐT (Haravan) trong app chính — Thiết kế

## Mục tiêu

App hiện chỉ xử lý được đơn siêu thị dạng PDF (Coop, BigC, Lotte,
Satra, Emart, Kingfood, Winmart, Fujimart, JMart, JIT). Đơn thương mại
điện tử (Shopee / TikTok Shop, đồng bộ về Haravan) đang được làm tay:
tải file Excel từ trang quản trị Haravan, dán vào workbook
`XUẤT HÀNG HN-LA MỚI.xlsx`, để công thức VLOOKUP quy đổi ra mã thành
phẩm, rồi chép sang `dondathang.xlsx` để import vào AMIS.

Mục tiêu: thêm **một nhánh xử lý nữa** trong app chính. Người dùng thả
workbook `XUẤT HÀNG HN-LA MỚI.xlsx` vào danh sách file rồi bấm đúng nút
**"Xử lý"** như mọi file khác; app hỏi khoảng thời gian cần lấy, gọi
thẳng Haravan Omni API, quy đổi mã thành phẩm, ghi kết quả vào sheet
`Haravan` của chính workbook đó **và** ghi dòng đặt hàng vào
`dondathang.xlsx` đúng khuôn mẫu AMIS. Mã nào chưa khai báo trong bảng
tra cứu thì hỏi người dùng ngay tại chỗ và ghi bổ sung trở lại bảng để
lần sau không hỏi lại.

## Bối cảnh kỹ thuật hiện tại

### App chính (`GO/`, module `order-processor`)

- `App.ProcessFiles(files, stt)` (`GO/app.go:436`) đặt chỗ batch rồi
  chạy `go a.runReservedBatch(...)`. Batch giữ `a.excelMu`, gọi
  `excelwriter.ClearOrderRows(a.excelPath)` **một lần** ở đầu để
  `dondathang.xlsx` chỉ chứa kết quả của lần chạy mới nhất, rồi lặp
  từng file gọi `a.processOne` và phát `process:log` / `process:row` /
  `process:progress` / `process:done`.
- `processing.RealProcessor.process`
  (`GO/internal/processing/coop_processor.go:63`) mở đầu bằng
  `extractPageTexts(filePath)` — **chỉ đọc được PDF**. Một file `.xlsx`
  đi vào đây sẽ rơi vào nhánh lỗi "không đọc được PDF".
- `fileset.IsAllowed` đã chấp nhận `.xlsx` sẵn, nên workbook TMĐT vào
  được danh sách file mà không phải sửa gì.
- `excelwriter.Row` + `writeRow`
  (`GO/internal/processing/excelwriter/dondathang.go:346`) ghi các cột
  A, B, C, D, E, G, K, L, Q, S, T, U, V, X, Y, Z, AE, AJ, AM, AN, AO,
  AP, AQ, AT, AU, AV của sheet `Don dat hang`. Đây đúng là tập cột đơn
  TMĐT cần — nhưng `writeRow` **luôn** ghi `Z` (công thức hoặc 0) và ghi
  `AT`/`AU` cho mọi dòng không phải dòng ghi chú, trong khi mẫu chuẩn
  TMĐT để trống cả ba ô đó.
- `productdata.Store.GetProductInfo(sku)` trả `ProductInfo{Name,
  WeightKg, PackSize}` — nguồn duy nhất cho cột `Tên hàng` (S) theo mã
  thành phẩm. Store nạp từ Google Sheets lúc `InitializeApp`.
- `appsettings.Settings{Gid, Zalo, Reminder map[string]string}` lưu ở
  `settings.bhconfig`, sửa qua popup bánh răng (`SettingsModal.tsx` +
  `KeyValueEditor.tsx`).

### Module Haravan rời (`GO/haravan/`, module `haravan-order-export`)

Chưa commit vào git (đang là thư mục untracked). Đã chạy thật và đã đối
chiếu: xuất lại khoảng 22–23/08/2026 rồi so từng ô với file người dùng
làm tay — **1.564/1.564 đơn, 1.585 dòng, trùng khớp 100%**.

- `internal/haravan`: `Client` (retry 429/5xx, tự giảm tốc theo header
  leaky-bucket), `ListOptions{CreatedAtMin, CreatedAtMax, ...}`,
  `ListOrders`, `CountOrders`, `Order`/`LineItem`, `ShopName(o)`,
  `VNLocation`, nhận diện sàn Shopee/TikTok (`channel.go`).
- `internal/lookup`: `Tables.Load(path)` đọc 2 sheet tra cứu,
  `ByCombo(sku)`, `ByProductVariant(title, variant)`, `MisaCode(shop)`,
  hằng `NotAvailable = "#N/A"`.
- `internal/export`: `StandardWriter` ghi bố cục 23 cột "chuẩn",
  `MissingCombo`/`MissingShop` đã thống kê sẵn những gì chưa khai báo.
- `cmd/haravan-export`: CLI, cờ mặc định `-channels shopee,tiktok`,
  `-status any`, `-exclude-shop "CLEVY VIỆT NAM"`.

### Workbook `XUẤT HÀNG HN-LA MỚI.xlsx` (bản người dùng cập nhật 25/08)

| Sheet | Vùng | Vai trò |
|---|---|---|
| `Mã misa` | `B1:D12` | B = Tên Kênh, C = KÊNH BÁN, D = Mã MISA. 10 shop. |
| `data shop` | `A1:K292` | A = Tên sản phẩm, B = Phân loại, C = Mã combo, D/F/H/J = MÃ TP 1..4, E/G/I/K = SLTP1..4. 291 dòng. |
| `Haravan` | `A1:A1` | Sheet **trống**, chờ app ghi vào. |

Bản trước có sheet `data shop` khai vùng `A1:K1048576` (cả triệu dòng
rỗng) và nặng 2,6 MB; bản mới 31 KB, vùng khai đúng dữ liệu thật. Hai
rủi ro "GetRows trả về cả triệu dòng" và "mở/ghi chậm vì XML 17 MB"
trong bản thiết kế nháp vì thế không còn.

### File mẫu vàng `đơn hàng/mẫu chuẩn.xlsx`

Sheet `Don dat hang`, 1.430 dòng dữ liệu thật của 22–23/08/2026, đủ cả
hai kho (HN 545 / LA 885) — chính là **đầu ra đúng** ứng với đầu vào là
sheet `Đơn hàng haravan` (1.585 dòng) trong workbook gốc của người dùng.
Đây là fixture đối chiếu của thiết kế này.

## Phạm vi

**Làm:**

1. Gộp module `GO/haravan` vào module chính.
2. Nhận diện workbook TMĐT trong danh sách file.
3. Modal chọn khoảng ngày, bật khi bấm "Xử lý".
4. Nhánh TMĐT trong batch: gọi API → quy đổi → ghi 2 file Excel.
5. Modal sửa mã thiếu, ghi bổ sung vào `data shop`.
6. Mục cấu hình token Haravan trong popup Cài đặt.
7. Golden test đối chiếu `mẫu chuẩn.xlsx`.

**Không làm:**

- Không gửi Zalo cho đơn TMĐT.
- Không đổi bất kỳ nhánh vendor PDF nào đang chạy.
- Không bổ sung shop thiếu vào sheet `Mã misa` (chỉ cảnh báo — người
  dùng chốt như vậy).
- Không đụng tới hai bố cục `haravan` / `full` của CLI.

## Kiến trúc

### Gộp module

`GO/haravan/` chưa commit nên di chuyển bây giờ không mất lịch sử gì.
Gộp vào module chính để chỉ còn **một** `go.mod`, **một** phiên bản
excelize (2.11.0 thay vì 2.9.1 lệch nhau), **một** lệnh `go test ./...`.

```
GO/internal/tmdt/
    haravan/      ← từ GO/haravan/internal/haravan  (client, types, channel)
    lookup/       ← từ GO/haravan/internal/lookup   (+ AppendComboRows)
    export/       ← từ GO/haravan/internal/export   (StandardWriter, ... — CLI dùng)
    mapping.go    ← MỚI: quy đổi đơn → dòng sheet Haravan + dòng dondathang
    sheet.go      ← MỚI: ghi sheet "Haravan" vào workbook đang có
    detect.go     ← MỚI: nhận diện workbook TMĐT
GO/cmd/haravan-export/   ← CLI cũ, import các package trên
GO/internal/processing/excelwriter/tmdt.go  ← MỚI: WriteTMDTRows
```

Đường dẫn import đổi từ `haravan-order-export/internal/...` thành
`order-processor/internal/tmdt/...`; `GO/haravan/go.mod`, `go.sum` và
`haravan-export.exe` bị xoá.

### Nhánh TMĐT nằm ở tầng app, không nằm trong RealProcessor

`Processor.Process(ctx, filePath, stt)` không có chỗ để nhận khoảng
ngày, cũng không có `Emitter` để bật modal và chờ người dùng trả lời.
Nhồi ba thứ đó vào interface sẽ làm hỏng tính thuần tuý (dễ test, không
UI) của toàn bộ họ processor vendor.

Vì vậy nhánh rẽ đặt trong `runReservedBatch`: với mỗi file, nếu
`tmdt.IsWorkbook(path)` đúng thì gọi `a.processTMDTFile(...)`, ngược lại
gọi `a.processOne(...)` như cũ. `RealProcessor` không sửa một dòng nào.

Tầng quy đổi (`tmdt/mapping.go`) nhận **đầu vào trung tính**
`[]OrderLine` chứ không nhận `*haravan.Order` trực tiếp, để golden test
nạp đầu vào từ sheet `Đơn hàng haravan` của file thật mà không cần
mạng.

## Luồng chạy đầu-cuối

```
Thả / chọn file  ──► danh sách file (không hỏi gì, không đổi hành vi cũ)

Bấm "Xử lý"
   │
   ├─ frontend gọi InspectTMDTFiles(files) → danh sách file TMĐT
   │     có file TMĐT chưa chọn khoảng ngày → bật modal chọn ngày
   │     (huỷ modal = huỷ luôn lần bấm "Xử lý", không chạy gì)
   │
   └─ ProcessFiles(files, stt, ranges)   ← thêm tham số thứ 3
         ClearOrderRows(dondathang.xlsx)          (như hiện tại)
         với từng file:
            ├─ PDF vendor → processOne(...)       (nhánh cũ, nguyên vẹn)
            └─ workbook TMĐT → processTMDTFile(...)
                  1. lookup.Load(path)            2 bảng tra cứu
                  2. haravan.ListOrders(...)      theo khoảng đã chọn
                     phát process:log tiến độ mỗi trang
                  3. mapping.Build(...)           quy đổi, gom mã thiếu
                  4. còn mã thiếu?
                        phát tmdt:missing  ──► modal sửa
                        chờ trên channel   ◄── ResolveTMDTMissing / CancelTMDTMissing
                        lookup.AppendComboRows(path, ...)  ghi vào "data shop"
                        nạp lại bảng, quy đổi lại
                  5. tmdt.WriteHaravanSheet(path, rows)
                  6. excelwriter.WriteTMDTRows(dondathang.xlsx, rows)
                  7. phát process:row các dòng tóm tắt
```

### Chờ người dùng giữa batch

Batch chạy trong goroutine và đang giữ `a.excelMu`. Bước 4 là chỗ
**duy nhất** trong toàn app dừng lại chờ người dùng khi đang giữ khoá —
khác hẳn mọi nhánh vendor vốn chạy một mạch. Ba biện pháp giữ cho nó
không thành treo vĩnh viễn:

- `a.tmdtResolve chan tmdtResolution` có đệm 1; `ResolveTMDTMissing` và
  `CancelTMDTMissing` (hai method Wails mới) đều gửi vào đó.
- `select` kèm `time.After(10 * time.Minute)` — quá hạn coi như huỷ.
- `select` kèm `ctx.Done()` — đóng app là thoát ngay.

Huỷ (bấm Huỷ, quá hạn, hoặc đóng app) **không** bỏ qua các dòng thiếu:
chúng vẫn được ghi với `#N/A` ở cột mã hàng, kèm log cảnh báo. Bỏ âm
thầm vài trăm dòng khỏi một file dùng để hạch toán nguy hiểm hơn nhiều
so với một ô `#N/A` mà AMIS sẽ báo lỗi ngay khi import.

## Modal chọn khoảng ngày

- Lịch tự vẽ theo tông màu app (không dùng `<input type="date">` của
  WebView2 — giao diện lạc lõng và không chặn được theo ràng buộc dưới).
- `max = hôm qua` (giờ máy). Hôm nay và tương lai bị chặn: đơn hôm nay
  chưa chốt.
- Khoảng chọn tối đa **7 ngày** tính cả hai đầu. Khi đã chọn ngày đầu,
  mọi ngày cách quá 6 ngày bị vô hiệu hoá ngay trên lịch — chặn ở chỗ
  bấm chứ không báo lỗi sau khi bấm.
- Ba preset: *Hôm qua* · *3 ngày* · *7 ngày*.
- Khoảng thời gian gửi xuống backend dạng `{from: "2026-08-22", to:
  "2026-08-23"}`; backend đổi thành `00:00:00+07` ngày đầu →
  `23:59:59+07` ngày cuối (`haravan.VNLocation`), lọc theo `created_at`.
- Backend **kiểm lại** cả hai ràng buộc, không tin frontend.

## Modal sửa mã thiếu

Payload sự kiện `tmdt:missing`:

```go
type MissingCombo struct {
    Key       string // khoá gom nhóm: SKU, hoặc "title|variant" khi không có SKU
    Product   string // Tên sản phẩm  → cột A của "data shop"
    Variant   string // Phân loại     → cột B
    Combo     string // Mã sản phẩm   → cột C (rỗng nếu đơn không có SKU)
    LineCount int    // số dòng hàng đang vướng, để người dùng biết mức ảnh hưởng
}
```

- Gom **unique** theo `Key` — 300 dòng cùng thiếu một mã chỉ hỏi một lần.
- Mỗi mục hiện sẵn Tên sản phẩm / Phân loại / Mã combo (chỉ đọc), người
  dùng chỉ điền `MÃ TP 1..4` + `SLTP 1..4` — đúng hình dạng một dòng của
  `data shop`.
- Lưu → `lookup.AppendComboRows(path, rows)` ghi tiếp vào cột A–K ngay
  dưới dòng cuối có dữ liệu của `data shop`, rồi nạp lại bảng và quy đổi
  lại. Lần chạy sau không hỏi nữa.
- Bỏ trống một mục = giữ `#N/A` cho mục đó, các mục khác vẫn được ghi.

## Quy tắc quy đổi dữ liệu

Nguồn: các cột của Haravan Omni API; đích: sheet `Haravan` và
`dondathang.xlsx`. Một dòng hàng (`line_item`) sinh ra **một dòng
`dondathang` cho mỗi thành phẩm** mà nó quy đổi ra (combo 2 thành phẩm
→ 2 dòng). Không gộp trùng.

### Bộ lọc đơn

| Điều kiện | Giá trị |
|---|---|
| Khoảng ngày | `created_at` trong khoảng người dùng chọn |
| Sàn | `shopee`, `tiktok` (nhận diện theo `channel.go`) |
| Trạng thái | `any` — **đơn đã huỷ vẫn lấy** (mẫu chuẩn chứa 230 dòng có `Trạng thái hủy = Yes`) |

**Hai đích, hai mức lọc khác nhau** — chỗ này dễ nhầm nên nói rõ:

- Sheet `Haravan` giữ **mọi** dòng hàng Shopee/TikTok trong khoảng, kể
  cả shop `CLEVY VIỆT NAM` (169/1.585 dòng). Đơn CLEVY cố tình **không**
  quy đổi thành phẩm — hằng `shopKhongQuyDoi` trong
  `export/standard.go`, đúng như công thức cũ — nên 8 cột MÃ TP/SLTP của
  các dòng đó **để trống**.
- `dondathang.xlsx` chỉ sinh dòng cho những dòng hàng **có** mã thành
  phẩm. Vì vậy phải phân biệt hai loại ô trống:

  | Trường hợp | Sheet `Haravan` | `dondathang.xlsx` |
  |---|---|---|
  | Shop CLEVY — không quy đổi theo thiết kế | MÃ TP để **trống** | **không sinh dòng** |
  | Chưa khai báo trong `data shop` | MÃ TP = `#N/A` | sinh dòng, cột Q = `#N/A` (sau khi người dùng bỏ qua modal sửa) |

Kiểm chứng số học: 1.585 dòng hàng − 169 dòng CLEVY = 1.416, cộng 14
dòng sinh thêm từ combo nhiều thành phẩm = **1.430** — đúng bằng số dòng
của `mẫu chuẩn.xlsx`.

### Giá trị dẫn xuất

| Ký hiệu | Công thức |
|---|---|
| `kho` | `Kho bán` (`location_name`) = `"Kho Hà Nội"` → `HN`; còn lại → `LA` |
| `kênh` | `Shopee` \| `TikTok` (nhận diện sàn) |
| `shop` | `note_attributes["X-Haravan-SalesChannel-BranchName"]` |
| `ngày` | `created_at` đổi sang giờ VN, định dạng `dd/mm/yyyy` |
| `TPᵢ, SLᵢ` | tra `data shop`: có `Mã sản phẩm` → tra theo `Mã combo`; không có → tra theo `Tên sản phẩm + Phân loại` |

### Sheet `Haravan` (23 cột, bố cục "chuẩn" sẵn có)

Giữ nguyên `standardHeaders` của `export/standard.go`: `Mã đơn hàng`,
`Tổng tiền`, `Tổng cộng`, `Ngày đặt hàng`, `Số lượng sản phẩm`,
`Tên sản phẩm`, `Giá trị thuộc tính 1`, `Giá sản phẩm`, `Mã sản phẩm`,
`Thuộc tính`, `Kho bán`, `Kênh bán hàng`, `Thời gian Đặt`,
`MÃ TP 1..4`, `SLTP1..4`, `Shop`, `Mã misa` — một dòng cho mỗi dòng
hàng (không tách theo thành phẩm).

### `dondathang.xlsx`, sheet `Don dat hang`

| Cột | Tiêu đề | Giá trị |
|---|---|---|
| A | Ngày đơn hàng | `ngày` |
| B | Số đơn hàng | `ĐĐHTMĐT-{kênh}-{Mã đơn hàng}` |
| C | Trạng thái | `Chưa thực hiện` |
| D | Ngày giao hàng | `ngày` |
| E | Địa điểm giao hàng | `kho` |
| G | Mã khách hàng | tra `Mã misa` theo `shop`; không có → `#N/A` + cảnh báo log |
| L | Diễn giải | `TMĐT-{kênh} - {shop} - {Mã đơn hàng} - Ngày đổ {ngày} - {kho}` |
| Q | Mã hàng | `TPᵢ` |
| S | Tên hàng | `Store.GetProductInfo(TPᵢ).Name` |
| T | Là dòng ghi chú | `Không` |
| U | Hàng khuyến mại | `Có` khi `Giá sản phẩm = 0`, ngược lại `Không` |
| V | Mã kho | `HN` → `TP_HN_12`; `LA` → `LA_KHOTMDT` |
| X | Số lượng | `Số lượng sản phẩm × SLᵢ` |
| Y | Đơn giá | `Giá sản phẩm ÷ SLᵢ ÷ 1,08` |
| AE | % thuế GTGT | `8` |
| AJ | Mã đơn vị | `HN` → `TMĐT_MB`; `LA` → `TMĐT_MN` |
| AM | Mã thống kê | `kho` |
| AO | Ghi Chú | `Mã đơn hàng` |
| AV | Số ngày được nợ | `15` |
| Z, AT, AU | Thành tiền, Trọng lượng, (AU) | **để trống** |

Ba ô cuối là lý do cần hàm ghi riêng: `writeRow` hiện có luôn ghi `Z`
(công thức `=Y*X` hoặc số 0) và ghi `AT`/`AU` cho mọi dòng không phải
dòng ghi chú, còn mẫu chuẩn TMĐT để trống cả ba. Thêm ba cờ phủ định
nữa vào `excelwriter.Row` — vốn đã mang 6 biệt lệ riêng của từng vendor
— sẽ làm struct đó khó đọc hơn là đáng. Nên thêm
`excelwriter.WriteTMDTRows(path, rows []TMDTRow)` dùng chung phần
"mở file / tìm dòng kế tiếp / lưu" với `WriteOrderRows`.

Xác minh công thức Y và X trên đơn nhiều dòng `2608235QED370T` (tổng
139.000, dòng 2 là hàng tặng giá 0): `139000 ÷ 1 ÷ 1,08 = 128.703,7037`
khớp mẫu chuẩn, dòng tặng ra `Y = 0` và `U = Có`. Chiết khấu ở cấp đơn
hàng (`Số tiền giảm`) **không** tham gia công thức.

## Bảng kết quả và log

Đơn TMĐT **không** đổ từng dòng vào bảng kết quả: một tuần có thể ~2.500
đơn / ~5.000 dòng, vừa làm ngập bảng vừa phá cơ chế tick chọn PO để gửi
Zalo. Thay vào đó phát **một `process:row` tóm tắt cho mỗi (shop, ngày)**
— vài chục dòng, đủ soát:

- `System` = `TMĐT-{kênh}`, `PO` = `{shop} · {ngày}`, `MaKhachHang` = mã
  MISA, `Page` = `{kho}`, `Status` = số đơn / số dòng đã ghi.
- Dòng tóm tắt có `StatusKind` = `warning` khi nhóm đó còn `#N/A`.

Tiến độ tải API đi qua `process:log` (theo từng trang 50 đơn) để người
dùng thấy app đang chạy chứ không đứng hình.

## Cấu hình

Thêm khối `<haravan>` vào `settings.bhconfig` — cùng cơ chế
`map[string]string` như `gid`/`zalo`/`reminder`, nên popup Cài đặt chỉ
cần thêm một tab dùng lại `KeyValueEditor.tsx`:

```
haravan.access_token = <private token, scope com.read_orders>
haravan.exclude_shops = CLEVY VIỆT NAM
```

- Token **không bao giờ** đi vào `process:log` hay bất kỳ log nào khác.
- Thiếu token → nhánh TMĐT dừng sớm với thông báo chỉ rõ chỗ điền, các
  file PDF khác trong cùng batch vẫn chạy bình thường.

## API và sự kiện mới

| Hướng | Tên | Nội dung |
|---|---|---|
| Go → JS | `tmdt:missing` | `[]MissingCombo` |
| JS → Go | `InspectTMDTFiles(paths []string) ([]string, error)` | lọc ra file TMĐT |
| JS → Go | `ProcessFiles(files []string, stt int, ranges map[string]DateRange)` | **đổi chữ ký**, thêm tham số thứ 3 |
| JS → Go | `ResolveTMDTMissing(entries []ComboEntry) error` | |
| JS → Go | `CancelTMDTMissing() error` | |

`ProcessFiles` chỉ có một chỗ gọi (`ControlPanel.tsx`) nên việc đổi chữ
ký gọn; batch không có file TMĐT thì truyền map rỗng.

Hai struct đi kèm:

```go
type DateRange struct {
    From string `json:"from"` // "2026-08-22", giờ VN
    To   string `json:"to"`   // "2026-08-23", tính hết ngày
}

// ComboEntry là một dòng người dùng vừa khai trong modal sửa mã thiếu —
// đúng hình dạng một dòng cột A..K của sheet "data shop".
type ComboEntry struct {
    Key     string    `json:"key"`     // khớp MissingCombo.Key
    Product string    `json:"product"` // cột A
    Variant string    `json:"variant"` // cột B
    Combo   string    `json:"combo"`   // cột C
    TP      [4]string `json:"tp"`      // cột D, F, H, J
    SL      [4]string `json:"sl"`      // cột E, G, I, K
}
```

Dòng tóm tắt TMĐT tiêu thụ số thứ tự (`stt`) như mọi `process:row` khác,
nên `config.txt` sau batch vẫn tăng đúng bằng tổng số dòng đã phát.

## Kiểm thử

1. **Golden test quy đổi** (`tmdt/mapping_test.go`) — nạp fixture đầu
   vào lấy từ sheet `Đơn hàng haravan` của workbook thật (22–23/08,
   1.585 dòng) + 2 bảng tra cứu, chạy `mapping.Build`, so **từng ô** với
   1.430 dòng của `đơn hàng/mẫu chuẩn.xlsx`. Đây là bài kiểm tra quyết
   định: đạt nghĩa là toàn bộ quy tắc ở trên đúng.
2. **Ràng buộc modal ngày** (`lib/tmdtDateRange.test.ts`) — chặn hôm
   nay/tương lai, chặn khoảng > 7 ngày, các preset ra đúng khoảng.
3. **`lookup.AppendComboRows`** — ghi đúng cột A–K, đúng dòng kế tiếp,
   không đụng dòng có sẵn; nạp lại bảng thì tra được ngay.
4. **`excelwriter.WriteTMDTRows`** — Z/AT/AU trống, các cột khác đúng,
   ghi nối tiếp sau dòng cuối (không đè dòng vendor PDF cùng batch).
5. **Nhận diện workbook** — file có đủ 3 sheet là TMĐT; workbook khác và
   PDF thì không.
6. **Chờ/huỷ** — fake `Emitter` + gọi `CancelTMDTMissing`, kiểm rằng
   nhánh ghi `#N/A` và batch kết thúc chứ không treo.
7. **Không hồi quy** — `go test ./...` của toàn bộ golden suite vendor
   (151/151) phải vẫn xanh sau khi gộp module.

Test gọi mạng thật bị loại: `haravan.Client` nhận `*http.Client` nên
test dùng transport giả, đúng cách `client_test.go` hiện có đang làm.

## Rủi ro

- **Đổi chữ ký `ProcessFiles`** buộc phải sinh lại Wails bindings; quên
  bước này thì frontend gọi vào bản cũ và tham số `ranges` biến mất âm
  thầm. Sinh lại bindings nằm trong bước triển khai đầu tiên.
- **Gộp module** đưa excelize của phần Haravan từ 2.9.1 lên 2.11.0.
  Rủi ro thấp (cùng nhánh v2) nhưng phải chạy lại test của `export`
  ngay sau khi gộp, trước khi viết code mới.
- **Workbook đang mở trong Excel** → không ghi được. Báo lỗi rõ ("đóng
  file rồi thử lại"), không ghi nửa vời — cùng cách `ClearOrderRows`
  đang xử lý.
- **`ClearOrderRows` chạy một lần cho cả batch**: nếu người dùng chọn
  chung file TMĐT với file PDF vendor, cả hai cùng ghi nối tiếp vào
  `dondathang.xlsx`. Đây là hành vi mong muốn, nhưng thứ tự dòng phụ
  thuộc thứ tự file trong danh sách — cần nói rõ trong log.

## Quyết định đã chốt với người dùng

| Điểm | Chốt |
|---|---|
| Vị trí nhánh | Trong app chính `GO/` |
| Kích hoạt | Bấm nút "Xử lý", modal ngày bật lúc đó |
| Sheet đích | `Haravan`, ghi đè mỗi lần chạy |
| `dondathang.xlsx` | Xoá sạch rồi ghi (như batch hiện tại) |
| Token | Mục mới trong Cài đặt (`settings.bhconfig`) |
| Phạm vi #N/A | Chỉ mã thành phẩm (`data shop`); thiếu shop chỉ cảnh báo |
| Bảng kết quả | Chỉ dòng tóm tắt |
