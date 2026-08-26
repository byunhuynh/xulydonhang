# Thiết kế: tích hợp push MISA vào app (nhánh Hà Thành / HTLA)

## Bối cảnh

Thư mục `misa/` ở gốc repo là một module Go độc lập gồm hai công cụ dòng lệnh:

- `misasniff` — mở Chrome, nghe toàn bộ network của `actapp.misa.vn`, dựng lại
  tài liệu API + Go client + file phiên đăng nhập.
- `misapush` — dựng lại đúng luồng **nhập khẩu chứng từ từ Excel** của AMIS Kế
  toán (5 bước: upload → sheetname → step2 → step3 → step4) bằng dòng lệnh.

Luồng đó đã chạy thật và đã có bộ test khoá lại hành vi (`misa/PUSH.md`,
`misa/internal/misa/*_test.go`). Việc còn thiếu là đưa nó vào app Wails để người
dùng không phải mở terminal.

Ba dữ kiện quyết định hình dạng thiết kế, đều đã kiểm chứng trực tiếp:

1. **`dondathang.xlsx` ở gốc repo CHÍNH LÀ file mẫu nhập khẩu của MISA.** Đối
   chiếu byte-level với `misa/dondathang.xlsx`: cùng sheet `Don dat hang`, cùng
   khối hướng dẫn dòng 1–7, cùng hàng tiêu đề dòng 8 (48 cột, `Ngày đơn hàng (*)`
   … `Số ngày được nợ`), dữ liệu từ dòng 9. App xử lý xong là đẩy thẳng được,
   **không cần bước chuyển đổi định dạng nào**.

2. **`misa/internal/misa` chỉ dùng thư viện chuẩn.** Quét import của cả 10 file
   (6 file nguồn + 4 file test): `bufio bytes context encoding/base64
   encoding/binary encoding/json errors fmt io mime/multipart net/http
   net/http/httptest net/url os path/filepath sort strconv strings sync
   sync/atomic testing time unicode/utf16`. Không một import nào trỏ ra ngoài
   package (`grep -c "misasniff/"` = 0 ở cả 10 file). Copy sang
   `order-processor/internal/misa/` là chạy, `go.mod` **không đổi một dòng nào**.
   Đây đúng cách `haravan/` đã được nhập thành `GO/internal/tmdt/{export,haravan,
   lookup}` ở nhánh TMĐT.

3. **`excelize.RemoveRow` tự chỉnh lại công thức tương đối.** Đã dựng thử trên
   chính `dondathang.xlsx`: 6 dòng dữ liệu r9–r14, mỗi dòng mang
   `Z{r} = Y{r}*X{r}`; xoá r14, r12, r10 từ dưới lên; đọc lại được r9/r10/r11
   mang `Y9*X9` / `Y10*X10` / `Y11*X11` — đúng chỉ số mới. Khối tiêu đề dòng 1–8
   và ô gộp `Q7:AP7` còn nguyên. Nên **tách file theo nhánh làm được bằng cách
   copy file rồi xoá dòng thừa**, không phải dựng lại workbook từ đầu (việc dựng
   lại sẽ phải chép thủ công style, ô gộp, độ rộng cột — nguồn sai lệch không cần
   thiết).

## Quyết định của người dùng (khoá lại, không suy diễn)

| Câu hỏi | Quyết định |
|---|---|
| Chọn nhánh lúc nào | Map sẵn theo khoá định tuyến trong Cài đặt; lúc push vẫn chỉnh tay được **từng đơn** |
| Phạm vi push | **Toàn bộ bảng kết quả** vừa xử lý — mọi đơn đều vào modal, bỏ bớt bằng ô tick **của riêng modal**; không liên quan tới ô tick chọn nhóm Zalo trên bảng |
| Bảng trống | Nút Push **vô hiệu hoá** |
| Phiên đăng nhập | File `misa-session.json` + `sid_url` (Apps Script) khai trong Cài đặt |
| Ghi sổ | **Một bước**: kiểm tra xong, không dòng nào lỗi thì ghi luôn |
| Số lần đẩy | **Một nhánh = một file = một lần đẩy.** Tối đa 2 lần cho cả lô |
| Kiểu điều khiển chọn nhánh | **Segmented control** giống bộ chọn buổi của JIT, không dùng dropdown |
| Định tuyến mặc định | HTLA: TMĐT, COOP, DIY, LOTTE, SATRA, **BigC gia công**. Hà Thành: BIGC (modern trade), EMART, WINMART, KINGFOOD. **JIT tách theo kho**: `WH6_HN` → Hà Thành, `WH6_HTLA` → HTLA |

## Kiến trúc

```
┌─ frontend ────────────────────────────────────────────────────────┐
│ ControlPanel  ── nút "Push MISA" (bật khi rows.length > 0)        │
│      │                                                            │
│      v                                                            │
│ MisaPushModal ── 1 dòng / 1 đơn: ☑ | Số ĐH | nhãn khoá | dòng |   │
│                  SegmentedControl[ Hà Thành │ HTLA ]              │
│      │ PushMisa(jobs)                                             │
└──────┼────────────────────────────────────────────────────────────┘
       v
┌─ app.go ──────────────────────────────────────────────────────────┐
│ (a *App) PushMisa(jobs []MisaPushJob)   ← chạy nền, phát sự kiện  │
│      │ gom excelRows theo branch (RouteKey → nhánh)               │
│      v                                                            │
│ internal/misapush ── SplitWorkbook(src, dst, keep []int)          │
│                   └─ Pusher.Push(ctx, Request) → *misa.ImportResult│
│      │                                                            │
│      v                                                            │
│ internal/misa ── Client: LoginWithSession → SwitchDatabaseByName   │
│                          → ImportExcel{Commit:true, Force:false}   │
└───────────────────────────────────────────────────────────────────┘
```

## Thành phần

### 1. `GO/internal/misa/` — nhập nguyên thư viện

Copy 10 file từ `misa/internal/misa/` sang, đổi đúng **không gì cả** (package đã
tên `misa`, không import chéo). Bộ test đi theo, giữ nguyên giá trị: nó khoá lại
tham số từng bước của luồng 5 bước, việc bắn lại nguyên vẹn bản đồ cột sang
step3/step4, chế độ chạy thử không gọi step4, vòng chờ worker tới `end=true`, lỗi
khi thiếu cột bắt buộc, chặn ghi sổ khi còn dòng không hợp lệ, và yêu cầu
`X-Device` khi đổi bộ dữ liệu.

`misa/` ở gốc repo **giữ nguyên** làm công cụ dòng lệnh và nơi chạy `misasniff`
khi MISA đổi API. Không xoá, không biến thành symlink — hai bản sao là có chủ ý:
một bản là công cụ khảo sát, một bản là thư viện của app.

### 2. `GO/internal/misapush/split.go` — tách file theo nhánh

```go
// SplitWorkbook copy src sang dst rồi xoá mọi dòng dữ liệu (từ firstDataRow)
// không nằm trong keep. Khối mẫu nhập khẩu của MISA (dòng 1..firstDataRow-1)
// giữ nguyên, kể cả ô gộp và style.
func SplitWorkbook(src, dst string, keep []int) error
```

- `firstDataRow = 9`, `sheetName = "Don dat hang"` — hằng số nội bộ, khớp
  `excelwriter.ClearOrderRows` (nó xoá từ dòng 9 trở đi, cùng một quy ước).
- Xoá **từ dưới lên** để chỉ số dòng phía trên không xê dịch giữa chừng.
- `keep` rỗng → trả lỗi, không tạo file rác.
- Chỉ số ngoài phạm vi dữ liệu → trả lỗi (dấu hiệu `ExcelRows` lệch với workbook,
  đáng dừng lại chứ không đẩy nhầm dòng).

### 3. `GO/internal/misapush/push.go` — một lần đẩy cho một nhánh

```go
type Request struct {
    SessionPath string // misa-session.json
    SidURL      string // Apps Script cấp phiên mới; rỗng = không tự gia hạn
    Database    string // tên hoặc database_id bộ dữ liệu kế toán
    FilePath    string // file .xlsx đã tách cho nhánh này
    Log         func(string)
}

type Pusher interface {
    Push(ctx context.Context, req Request) (*misa.ImportResult, error)
}
```

`HTTPPusher` là bản thật:

1. `misa.NewClient("")`
2. `SetRenewFromURL(req.SidURL, save)` **trước** khi đăng nhập — phiên trong file
   chết thì client tự xin phiên mới rồi ghi đè file (0600) thay vì bắt người dùng
   chạy tay `misasniff`.
3. `LoadSession(req.SessionPath)` → `UseSession`; đọc không được mà có `SidURL`
   thì bỏ qua, để nhánh Renew lo.
4. `Login(ctx)` — cấp token ngay để phát hiện phiên hỏng **trước** khi upload,
   thay vì để lỗi nổ giữa lúc đang đẩy dữ liệu.
5. `SwitchDatabaseByName(ctx, req.Database)`
6. `ImportExcel(ctx, ImportOptions{FilePath: …, RefType: RefTypeSAOrder,
   TableName: TableSAOrder, SheetIndex: -1, Commit: true, Force: false})`

**Mỗi nhánh dùng một `Client` mới.** Lý do: `Client` giữ `Headers` biến đổi
(`Authorization`, `X-MISA-Context`) và `SwitchDatabase` thay `X-MISA-Context` tại
chỗ. Đổi bộ dữ liệu hai lần trong cùng một client là đường chưa ai đi — `misapush`
chỉ đổi đúng một lần mỗi lần chạy. Một client mỗi nhánh tái hiện chính xác một
lần chạy `misapush` đã được kiểm chứng. Giá phải trả là một lời gọi
`login/misa_id` thêm; `misa/PUSH.md` đã xác nhận cấp token mới **không giết**
token đang có, nên vô hại.

**`Commit: true, Force: false`** cho ra đúng hành vi người dùng chọn: MISA đọc và
kiểm tra cả file ở step3, không dòng nào lỗi thì step4 ghi sổ luôn; còn dòng lỗi
thì `ImportExcel` dừng lại và trả lỗi **kèm `*ImportResult`** — nên app liệt kê
được đủ `res.RowErrors`, không chỉ dòng đầu như thông điệp lỗi. Cả nhánh không
ghi gì, không có chuyện vào sổ nửa lô rồi phải dò xem đơn nào đã lọt.

### 4. Khoá định tuyến — `misapush.RouteKey`

Hệ thống một mình **không đủ** để định tuyến. Hai chỗ phải tách nhỏ hơn, cả hai
đều là yêu cầu thật của người dùng và đều đọc được từ dữ liệu đã có trên
`OrderRow`:

| Hệ thống | Khoá định tuyến | Nguồn phân biệt |
|---|---|---|
| `JIT-CHOICE` | `JIT-CHOICE/WH6_HN`, `JIT-CHOICE/WH6_HTLA` | `OrderRow.ShipTo` — `jit_airway_processor.go` gán thẳng mã kho bóc từ tên file (`air_waybill_WH6_HTLA_24082026.pdf`) |
| `BigC` | `BigC/GC`, `BigC/MT` | phân khúc mã KH — `bigc.ResolveCustomerCode` chỉ sinh ra đúng 4 mã: `MB_GC_BIGC`, `MB_MT_BIGC`, `MN_GC_BIGCAC`, `MN_MT_BIGCAC` |
| còn lại | chính `OrderRow.System` | — |

```go
// RouteKey trả về khoá tra bảng định tuyến cho một dòng kết quả.
func RouteKey(system, customerCode, shipTo string) string
```

Đặt trong `internal/misapush` chứ không phải `internal/processing`: đây là quy
tắc **kế toán** (đơn này vào sổ của pháp nhân nào), không phải quy tắc xử lý đơn.
`processing` không được biết gì về nhánh MISA.

Phân khúc mã KH lấy phần giữa của `<miền>_<phân khúc>_<mã NCC>`, viết hoa — đúng
phép tách mà `zalosend.splitCustomerCode` đã dùng. Mã không đủ 3 phần thì phân
khúc rỗng và khoá rơi về `BigC` trần (không bao giờ xảy ra với BigC thật, nhưng
không được panic).

**Giá trị mặc định gieo sẵn** (`misapush.SeedRouting`) — dùng khi
`misa_routing` chưa có khoá đó, ghi vào Cài đặt ngay lần chạy đầu:

| → HTLA | → Hà Thành |
|---|---|
| `TMĐT-*` (mọi sàn, khớp tiền tố) | `BigC/MT` |
| `COOPMART`, `COOPFOOD`, `Coop` | `Emart` |
| `Lotte` | `Winmart` |
| `Satra` | `Kingfood` |
| `MR.DIY` | `JIT-CHOICE/WH6_HN` |
| `BigC/GC` | |
| `JIT-CHOICE/WH6_HTLA` | |

`FujiMart`, `JMart` và mọi hệ thống chưa nêu **cố tình để trống** — người dùng
mới liệt kê tới đó, đoán thay là đoán vào sổ kế toán. Chúng hiện trong Cài đặt để
chọn, và modal push chặn đẩy nếu gặp một khoá chưa map.

`TMĐT-*` là **trường hợp tiền tố duy nhất**: tên sàn do `haravan.DetectChannel`
dò ra nên không liệt kê hết được (`TMĐT-Shopee`, `TMĐT-TikTok Shop`, sàn mới mai
sau). Tra theo thứ tự: khớp đúng khoá trước, không có thì thử tiền tố `TMĐT-`.
Mọi hệ thống khác chỉ khớp đúng.

### 5. Cấu hình — `appsettings.Settings` thêm 2 map

```go
Misa        map[string]string `json:"misa"`         // sid_url, db_ha_thanh, db_htla
MisaRouting map[string]string `json:"misa_routing"` // RouteKey -> "ha_thanh" | "htla"
```

`ensureMaps` khởi tạo cả hai thành map rỗng khi nil (JSON marshal của map nil ra
`null`, frontend cần object thật). File `settings.bhconfig` cũ thiếu 2 khoá này
đọc lên vẫn chạy — `encoding/json` để nil, `ensureMaps` vá lại. Không cần
migration.

Định danh nhánh trong `misa_routing` là **`"ha_thanh"` / `"htla"`** (khoá ổn
định), tên bộ dữ liệu thật tra qua `Misa["db_ha_thanh"]` / `Misa["db_htla"]`.
Đổi tên bộ dữ liệu bên MISA chỉ phải sửa một chỗ, không phải sửa lại toàn bộ bảng
định tuyến.

### 6. Giao diện Cài đặt — 2 tab mới

| Tab | Component | Nội dung |
|---|---|---|
| **MISA** | `KeyValueEditor` sẵn có, `secretKeys={['sid_url']}` | 3 khoá: `sid_url`, `db_ha_thanh`, `db_htla`. Giá trị `db_*` khớp một phần tên (không phân biệt hoa thường) hoặc `database_id` đầy đủ — đúng cách `-company` của misapush tra, xem `misa.FindDatabase` |
| **MISA – Nhánh** | `MisaRoutingEditor` (mới) | Mỗi khoá định tuyến một dòng + `SegmentedControl[Hà Thành │ HTLA]`. Không gõ tay khoá |

```
Cài đặt > MISA – Nhánh
──────────────────────────────────────────────────
BigC · gia công          │ Hà Thành │*HTLA*│
BigC · modern trade      │*Hà Thành*│ HTLA │
COOPFOOD                 │ Hà Thành │*HTLA*│
COOPMART                 │ Hà Thành │*HTLA*│
Emart                    │*Hà Thành*│ HTLA │
FujiMart                 │ Hà Thành │ HTLA │   ← chưa đặt
JIT · kho WH6_HN         │*Hà Thành*│ HTLA │
JIT · kho WH6_HTLA       │ Hà Thành │*HTLA*│
JMart                    │ Hà Thành │ HTLA │   ← chưa đặt
Kingfood                 │*Hà Thành*│ HTLA │
Lotte                    │ Hà Thành │*HTLA*│
MR.DIY                   │ Hà Thành │*HTLA*│
Satra                    │ Hà Thành │*HTLA*│
TMĐT-* (mọi sàn)         │ Hà Thành │*HTLA*│
Winmart                  │*Hà Thành*│ HTLA │
──────────────────────────────────────────────────
```

Nhãn hiển thị do frontend dựng từ khoá (`BigC/GC` → "BigC · gia công",
`JIT-CHOICE/WH6_HN` → "JIT · kho WH6_HN"), khoá lưu xuống file vẫn là chuỗi máy
đọc. Sắp xếp theo nhãn để danh sách không nhảy chỗ khi có khoá mới.

**Danh sách khoá lấy từ đâu.** Không hardcode hết được: `OrderRow.System` của Coop
lấy từ cột A sheet MAKH (`GetSystemForCustomer` → `COOPMART`/`COOPFOOD`), của TMĐT
là `"TMĐT-" + tên sàn` do `haravan.DetectChannel` dò ra. `MisaRoutingEditor` hiển
thị **hợp của** `misapush.SeedRouting` (bảng ở mục 4) **và** mọi khoá đã lưu trong
`misa_routing`; danh sách tự đầy lên nhờ cơ chế ghi nhớ ở mục 7.

### 7. Modal push

Nút "Push MISA" ở `ControlPanel` thay chỗ nhãn `SẮP RA MẮT` hiện có. Bật khi
`rows.length > 0 && !isProcessing && !isPushing`.

```
Push MISA — 12 đơn
──────────────────────────────────────────────────────────────
 ☑  3105551282   BigC · gia công       14 dòng │ Hà Thành │*HTLA*│
 ☑  4102277318   BigC · modern trade    9 dòng │*Hà Thành*│ HTLA │
 ☑  SO-99120     Lotte                  6 dòng │ Hà Thành │*HTLA*│
 ☑  air_waybill  JIT · kho WH6_HN      31 dòng │*Hà Thành*│ HTLA │
 ☐  2608258E3T   TMĐT-Shopee            3 dòng │ Hà Thành │*HTLA*│
 ☑  DH-7781      FujiMart               4 dòng │ Hà Thành │ HTLA │ ← chưa map
──────────────────────────────────────────────────────────────
 ☑ Ghi nhớ nhánh đã chọn
Hà Thành: 8 đơn / 96 dòng · HTLA: 3 đơn / 21 dòng
                                    [Huỷ]  [ĐẨY LÊN MISA]
```

- Gom nhóm bằng đúng `groupKeyFor` (`lib/zaloGrouping.ts`) mà bảng kết quả và nút
  Zalo đang dùng — một đơn trên modal là đúng một đơn người dùng thấy trên bảng.
- Cột thứ ba hiện **nhãn của khoá định tuyến**, không phải tên hệ thống trần —
  nhìn là biết vì sao hai đơn BigC lại rơi vào hai nhánh khác nhau.
- Nhánh mặc định: `RouteKey(row)` → tra `misa_routing` (khớp đúng, không phân biệt
  hoa thường; riêng `TMĐT-*` có nhánh khớp tiền tố) → không có thì tra
  `SeedRouting` → vẫn không có thì để trống.
- Khoá chưa map → **không nút nào sáng**. Nút "ĐẨY LÊN MISA" khoá cho tới khi mọi
  đơn đang tick đều có nhánh. Không đoán bừa vào Hà Thành.
- **"Ghi nhớ nhánh đã chọn"** (mặc định bật) ghi ngược `RouteKey → branch` vào
  `misa_routing` qua `SaveAppSettings`. Đây là cơ chế giữ cho bảng định tuyến đầy
  đủ mà không cần danh sách khoá cứng: sàn TMĐT mới hoặc phân khúc Coop mới xuất
  hiện lần đầu thì chọn tay một lần, từ lần sau tự có sẵn và hiện luôn trong Cài
  đặt.
- Hai đơn cùng một khoá định tuyến mà người dùng đổi nhánh của một đơn → **chỉ đơn
  đó đổi**, khoá không đổi theo. "Ghi nhớ" khi đó ghi nhánh của đơn cuối cùng
  người dùng chạm vào cho khoá ấy; nếu một khoá bị đặt hai nhánh khác nhau trong
  cùng một lượt thì **không ghi nhớ khoá đó** và log một dòng cảnh báo — thà để
  lần sau hỏi lại còn hơn ghi sai vào Cài đặt.

### 8. `SegmentedControl` dùng chung

Tách phần thân `JITPeriodMenu` thành `frontend/src/components/SegmentedControl.tsx`:

```tsx
interface SegmentedControlProps {
  options: readonly { value: string; label: string }[]
  value: string
  disabled?: boolean
  onChange: (value: string) => void
  ariaLabel: string
}
```

`JITPeriodMenu` trở thành lớp bọc mỏng gọi nó với `JIT_PERIOD_OPTIONS`. Giữ
nguyên `role="group"` + `aria-pressed` và **toàn bộ class hiện có** — kể cả lý do
đã ghi trong comment của `JITPeriodMenu` (nút hành động chứ không phải radiogroup,
vì bấm là ghi thẳng vào Excel; ở MISA thì bấm chỉ đổi trạng thái cục bộ của modal,
nhưng `role="group"` + `aria-pressed` vẫn đúng cho cả hai). Buổi JIT không đổi một
pixel, và nhánh MISA giống nó vì **dùng chung đúng một component**, không phải
chép lại style.

### 9. `App.PushMisa` — điều phối

```go
type MisaPushJob struct {
    PO        string `json:"po"`
    System    string `json:"system"`
    Branch    string `json:"branch"`    // "ha_thanh" | "htla"
    ExcelRows []int  `json:"excelRows"`
}

func (a *App) PushMisa(jobs []MisaPushJob)
```

Chạy nền như `SendZaloMessages`. `App` thêm `misaPusher misapush.Pusher` (mặc
định `HTTPPusher`, thay được trong test — cùng khuôn với `zaloSender` và
`dataLoader`) và `pushing atomic.Bool`.

Trình tự:

1. Từ chối nếu `a.processing.Load()` hoặc `!a.pushing.CompareAndSwap(false, true)`
   — đẩy trong lúc đang ghi workbook sẽ đọc phải file dở dang.
2. Đọc `settings.Misa`; thiếu `db_*` của nhánh đang cần → dừng nhánh đó với thông
   điệp chỉ thẳng vào Cài đặt.
3. Gom `ExcelRows` của mọi job theo `Branch`, sắp xếp tăng dần, loại trùng.
4. **Chạy tuần tự từng nhánh** (không song song: hai lần `login/misa_id` cùng lúc
   là hành vi chưa khảo sát, và log đan xen sẽ không đọc được):
   `SplitWorkbook` → file tạm trong `os.TempDir()` → `misaPusher.Push` →
   xoá file tạm.
5. Nhánh lỗi **không chặn** nhánh còn lại; báo kết quả riêng từng nhánh ở cuối.

Sự kiện:

| Sự kiện | Dữ liệu | Frontend làm gì |
|---|---|---|
| `misa:log` | `string` | Nối vào LogPanel sẵn có |
| `misa:pushed` | `{branch, ok, valid, invalid, message}` | Đánh dấu kết quả **theo nhánh** (một nhánh là một lần đẩy nguyên khối, không có kết quả riêng cho từng đơn) |
| `misa:done` | — | `setPushing(false)` |

Modal **không tự đóng** khi xong: nó chuyển sang màn hình kết quả, mỗi nhánh một
dòng ("Hà Thành: đã ghi 96 dòng" / "HTLA: 2 chứng từ lỗi, chưa ghi gì"), người
dùng đọc rồi tự đóng. Nhánh đã ghi thành công bị **bỏ tick và khoá lại**, nên bấm
đẩy lại ngay trong modal chỉ gửi các nhánh còn lỗi — không có đường nào ghi trùng
nhánh đã vào sổ. Ô tick chọn nhóm Zalo trên bảng kết quả **không bị đụng tới**.

## Xử lý lỗi

| Tình huống | Hành vi |
|---|---|
| `misa-session.json` không có + `sid_url` rỗng | Dừng trước khi upload, log chỉ dẫn khai `sid_url` trong Cài đặt hoặc chạy `misasniff -refresh-session` |
| Phiên chết, `sid_url` có | Client tự xin phiên mới, ghi đè file (0600), chạy tiếp — không cần người |
| Phiên chết, `sid_url` rỗng | Lỗi `ErrUnauthorized` kèm thông điệp có sẵn của thư viện |
| Token hết hạn giữa chừng | `Client.do` tự cấp lại đúng một lần rồi gửi lại chính request đó |
| Tên bộ dữ liệu khớp nhiều | `FindDatabase` liệt kê ra và dừng, không chọn bừa |
| Thiếu cột bắt buộc | `ImportExcel` dừng ở step2, log tên cột |
| Có dòng không hợp lệ | Không ghi gì cho nhánh đó; log **toàn bộ** `res.RowErrors` (dòng + số đơn + mô tả của MISA) |
| Nhánh 1 xong, nhánh 2 lỗi | Nhánh 1 đã vào sổ và được bỏ tick; nhánh 2 giữ tick để đẩy lại sau khi sửa |

## Bảo mật

- `misa-session.json` **không chứa mật khẩu nhưng thay được mật khẩu trong 24h**.
  Ghi quyền 0600 (`Session.Save` đã làm sẵn), thêm vào `.gitignore` gốc cùng khối
  với `zalo_state.json` và `zalo_profile/`.
- `sid_url` là URL Apps Script kèm `secret` trên query string — ai có nó lấy được
  phiên MISA. Vào `KeyValueEditor` với `secretKeys={['sid_url']}` (che thành chấm,
  chặn copy/cut) đúng như `haravan.access_token` đang làm.
- `settings.bhconfig` vốn đã nằm trong `.gitignore`.

## Kiểm thử

### Go

1. `internal/misa/*_test.go` — mang theo nguyên bộ test server giả đã có, chạy
   trong module `order-processor`. Đây là lưới an toàn cho toàn bộ luồng 5 bước.
2. `internal/misapush/split_test.go`
   - dựng workbook 5 dòng dữ liệu r9–r13, giữ `{9, 11, 13}`;
   - khẳng định dòng 1–8 giữ nguyên từng ô (so sánh với bản gốc);
   - khẳng định ô gộp `Q7:AP7` còn;
   - khẳng định 3 dòng còn lại là đúng 3 dòng đã chọn, đúng thứ tự;
   - khẳng định công thức `Z` hạ đúng chỉ số (`Y9*X9`, `Y10*X10`, `Y11*X11`);
   - `keep` rỗng → lỗi; chỉ số ngoài phạm vi → lỗi;
   - file nguồn **không bị sửa**.
3. `internal/misapush/route_test.go`
   - `RouteKey("JIT-CHOICE", "MN_JIT_01512", "WH6_HN")` → `JIT-CHOICE/WH6_HN`;
     `…"WH6_HTLA"` → `JIT-CHOICE/WH6_HTLA`;
   - `RouteKey("BigC", "MB_GC_BIGC", "")` → `BigC/GC`; cả 4 mã BigC thật
     (`MB_GC_BIGC`, `MB_MT_BIGC`, `MN_GC_BIGCAC`, `MN_MT_BIGCAC`) ra đúng 2 khoá;
   - `RouteKey("Lotte", "MN_MT_LOT1001", "")` → `Lotte` (hệ thống trần);
   - mã KH không đủ 3 phần với BigC → `BigC` trần, không panic;
   - JIT với `ShipTo` rỗng → `JIT-CHOICE` trần, không panic;
   - `Lookup` khớp không phân biệt hoa thường; `TMĐT-Shopee` và một sàn chưa
     từng thấy đều khớp nhánh tiền tố `TMĐT-`; `TMĐT-Shopee` đã lưu riêng trong
     `misa_routing` thì **khớp đúng thắng tiền tố**;
   - `SeedRouting` phủ đúng bảng ở mục 4 và **không** chứa `FujiMart`/`JMart`.
4. `internal/misapush/push_test.go` — `HTTPPusher` trỏ vào `httptest.Server` trả
   đúng phản hồi đã bắt được: khẳng định thứ tự gọi (login → database-context →
   upload → sheetname → step2 → step3 → step4) và `Force=false` chặn ghi khi có
   dòng lỗi.
5. `app_misa_test.go`
   - `PushMisa` gom đúng `ExcelRows` theo nhánh, loại trùng, sắp tăng dần;
   - đúng **một** lời gọi `Push` cho mỗi nhánh có đơn; **không** gọi cho nhánh rỗng;
   - từ chối khi `a.processing` đang bật;
   - từ chối lời gọi thứ hai khi `a.pushing` đang bật;
   - thiếu `db_htla` → chỉ nhánh Hà Thành chạy, log nêu rõ nhánh HTLA bị bỏ;
   - nhánh 1 lỗi vẫn chạy nhánh 2;
   - phát `misa:pushed` đúng nhánh, đúng cờ `ok`;
   - file tạm bị xoá kể cả khi `Push` trả lỗi.
6. `internal/appsettings/store_test.go` — thêm ca: `.bhconfig` cũ **không có**
   `misa`/`misa_routing` đọc lên ra map rỗng (không nil), ghi lại có đủ 6 khoá.

### Frontend (vitest)

7. `lib/misaBranch.test.ts`
   - gom nhóm đơn từ `rows` khớp đúng `groupKeyFor`;
   - nhãn hiển thị dựng đúng từ khoá (`BigC/GC` → "BigC · gia công",
     `JIT-CHOICE/WH6_HN` → "JIT · kho WH6_HN", `Lotte` → "Lotte");
   - tra nhánh mặc định: `misa_routing` thắng `SeedRouting`; khớp không phân biệt
     hoa thường;
   - khoá chưa map → nhánh rỗng;
   - tổng "x đơn / y dòng" mỗi nhánh tính đúng, chỉ đếm đơn đang tick;
   - `canPush` = false khi còn đơn đã tick mà chưa có nhánh, hoặc khi không tick
     đơn nào;
   - `rememberRouting` dựng đúng map `RouteKey → branch` từ lựa chọn hiện tại, và
     **bỏ qua** khoá bị đặt hai nhánh khác nhau trong cùng một lượt;
   - sau khi nhận `misa:pushed{branch, ok: true}`, nhánh đó bị loại khỏi lượt đẩy
     tiếp theo còn nhánh lỗi thì không.

## Cố ý không làm

- **Không** có nút "vẫn đẩy phần hợp lệ" (`-force`). Người dùng chọn một bước: có
  lỗi là dừng, sửa file rồi chạy lại. Thêm nút này là mở đường cho việc ghi thiếu
  đơn mà không ai để ý — đúng cái bẫy mà `misapush` cố tình chặn.
- **Không** nhúng đăng nhập Chrome/OTP vào app. Apps Script đã khép kín vòng lặp
  đó (`misa/appscript/Code.gs`), và app đã có một luồng chromedp (Zalo) là đủ.
- **Không** đụng `misa/` ở gốc repo, **không** đụng luồng xử lý đơn hiện tại,
  **không** đổi `excelwriter`.
- **Không** cho phép push khi bảng trống. Đẩy lại lô cũ đang nằm trên đĩa là đường
  ngắn nhất tới việc ghi trùng vào sổ kế toán.
- **Không** hỗ trợ loại chứng từ khác `3520 / sa_order`. Cùng luồng, chỉ đổi 2
  tham số — thêm khi thật sự cần.
