# Chọn giá PO hoặc giá hệ thống cho mã sai giá — Thiết kế

## Mục tiêu

Khi một sản phẩm bị đánh dấu "sai giá" (giá trên PO/hóa đơn khác giá hệ
thống tính ra), hiện tại app **luôn tự động** ghi giá hệ thống
(`finalPrice`) vào cột Y, tô đỏ, và thêm comment cảnh báo — không có cách
nào để người dùng chủ động giữ lại giá trên PO nếu họ xác nhận giá đó là
đúng/cố ý (giá thương lượng riêng, giá cũ trong sheet giá chưa cập nhật,
v.v).

Mục tiêu: sau khi xử lý xong, người dùng xem được **chi tiết từng mã sai
giá** (mã, tên, giá PO, giá hệ thống) ngay trong bảng kết quả, và với
**từng mã riêng lẻ** có thể chọn "Dùng giá PO" hoặc "Dùng giá hệ thống" —
chọn xong áp dụng ngay vào file Excel đã ghi, không cần xử lý lại file.

## Bối cảnh kỹ thuật hiện tại

- `RealProcessor.Process` (mỗi vendor qua `processXSegment`) xử lý đồng bộ:
  tính `matched`/`finalPrice`/`invoicePrice` cho từng sản phẩm, ghi thẳng
  vào Excel qua `excelwriter.WriteOrderRows` trước khi trả `OrderRow` về
  cho `app.go`. Không có bước chờ xác nhận nào.
- `excelwriter.WriteOrderRows(path string, rows []Row, headerDescription string) error`
  (`GO/internal/processing/excelwriter/dondathang.go:67`) tính
  `currentRow := len(existingRows) + 1` rồi `firstRow := currentRow`
  (dòng Excel thật của `rows[0]`) — **hiện KHÔNG trả giá trị này ra
  ngoài**, dù đã tính sẵn.
- Mỗi vendor, ngay tại vòng lặp sản phẩm, đã có `productRowIndex := len(rows)`
  (vị trí thật trong slice `rows` TRƯỚC khi append dòng sản phẩm đó — đã
  tính đúng, kể cả khi có dòng khuyến mãi xen giữa các sản phẩm) — dùng để
  ghi `PromoNote`/`PromoBundleSku` vào đúng dòng. Đây chính là offset cần
  để tính ra số dòng Excel thật, chỉ cần cộng với `firstRow`.
- Khi sai giá (`!matched`), mỗi vendor set `productRow.PriceMismatch = true`
  và `productRow.InvoicePrice = invoicePrice`; `excelwriter.writeRow`
  (dondathang.go:107) đọc 2 field này để tô đỏ ô Y và
  `f.AddComment(...)` — dùng `redFill` style đã tạo qua
  `f.NewStyle(&excelize.Style{Fill: ...})` trong `WriteOrderRows`.
- `OrderRow` (`GO/internal/processing/types.go`) đã có `SkuLog []string`
  (dòng log real-time, `json:"-"`, không lưu lại được) và
  `PriceMismatchCount int` (`json:"priceMismatchCount"`, chỉ là con số
  đếm) — **chưa có nơi nào lưu chi tiết per-SKU** (mã/tên/giá PO/giá hệ
  thống/số dòng Excel) để dùng lại sau khi `Process` đã trả về.
- `App` (`GO/app.go:38`) giữ `processor processing.Processor` — kiểu
  **interface**, không lộ field `ExcelPath` cụ thể của `RealProcessor`.
  Không có cách nào từ `App` truy cập đường dẫn Excel hiện tại ngoài việc
  thêm field riêng.
- `ResultTable.tsx` hiện 1 dòng = 1 `OrderRow` (1 file/trang), không có
  cơ chế mở rộng xem chi tiết.

## Kiến trúc

### 1. Dữ liệu chi tiết per-SKU (Go)

Thêm struct mới trong `types.go`:

```go
// PriceMismatchDetail là chi tiết MỘT sản phẩm bị đánh dấu sai giá,
// dùng để hiển thị lại cho người dùng chọn áp dụng giá PO hay giá hệ
// thống SAU khi Process đã trả về (không phải lúc xử lý).
type PriceMismatchDetail struct {
	SKU          string  `json:"sku"`
	ProductName  string  `json:"productName"`
	InvoicePrice float64 `json:"invoicePrice"` // giá trên PO/hóa đơn
	SystemPrice  float64 `json:"systemPrice"`  // giá hệ thống tính ra (finalPrice), giá ĐANG được ghi vào Excel
	ExcelRow     int     `json:"excelRow"`     // dòng thật trong sheet "Don dat hang", dùng để sửa lại sau này
}
```

Thêm field mới vào `OrderRow`:

```go
PriceMismatchDetails []PriceMismatchDetail `json:"priceMismatchDetails"`
```

(Khác `SkuLog` — field này CẦN serialize ra JSON để frontend dùng lại
sau khi nhận `process:row`.)

### 2. `excelwriter.WriteOrderRows` trả về dòng bắt đầu

Đổi signature:

```go
func WriteOrderRows(path string, rows []Row, headerDescription string) (startRow int, err error)
```

`startRow` chính là `firstRow` đã tính sẵn trong hàm — chỉ thêm vào giá
trị trả về, không đổi logic tính toán. Cập nhật cả 9 call site (mỗi
vendor 1 chỗ, xem danh sách file bên dưới) + 3 chỗ gọi trong
`excelwriter/dondathang_test.go`.

### 3. Mỗi vendor: gộp chi tiết mismatch + tính ExcelRow

Ngay tại chỗ mỗi vendor đã có `productRowIndex := len(rows)` và biết
`!matched` (chỗ hiện đang set `productRow.PriceMismatch`/`InvoicePrice`),
thêm:

```go
if !matched {
    productRow.PriceMismatch = true
    productRow.InvoicePrice = invoicePrice
    saigia++
    mismatchDetails = append(mismatchDetails, PriceMismatchDetail{
        SKU: barcode, ProductName: productInfo.Name,
        InvoicePrice: invoicePrice, SystemPrice: finalPrice,
        // ExcelRow tính SAU khi biết startRow (bước cuối hàm) — tạm lưu
        // productRowIndex, cộng bù ở bước ghép cuối.
    })
}
```

Vì `startRow` chỉ biết được SAU khi gọi `WriteOrderRows` (ở cuối hàm),
cách sạch nhất: mỗi vendor lưu tạm `productRowIndex` cùng chi tiết (một
slice `[]PriceMismatchDetail` với `ExcelRow` tạm = `productRowIndex`),
rồi NGAY SAU khi có `startRow` từ `WriteOrderRows`, cộng bù một lần:

```go
startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
if err != nil {
    return OrderRow{}, err
}
for i := range mismatchDetails {
    mismatchDetails[i].ExcelRow += startRow
}
```

Áp dụng đồng loạt cho cả 9 vendor đã ship: Coop, Lotte, Satra, BigC (qua
`storePageResult`, giống cách `skuLog`/`saigia` đã được thread qua đó),
Winmart, Emart, FujiMart, Kingfood, JMart.

### 4. Hàm sửa lại giá đã ghi (Go, package `excelwriter`)

```go
// ConfirmPrice ghi đè giá (cột Y) của một dòng ĐÃ TỒN TẠI trong sheet
// "Don dat hang", xóa tô đỏ và comment cảnh báo — dùng khi người dùng đã
// xem xét một mã sai giá và quyết định giữ giá nào (PO hoặc hệ thống).
// Yêu cầu ô Y{row} ĐANG có comment (tức đang thật sự bị đánh dấu sai
// giá) — nếu không, trả lỗi thay vì âm thầm ghi đè một ô không rõ trạng
// thái (ví dụ file đã bị sửa tay hoặc dòng đã được xử lý trước đó).
func ConfirmPrice(path string, row int, price float64) error
```

Logic: mở file, `GetComments` kiểm tra ô `Y{row}` đang có comment (nếu
không → lỗi rõ ràng "dòng này không còn ở trạng thái chờ xác nhận giá"),
`SetCellValue` ghi `price` vào `Y{row}`, `DeleteComment`, `SetCellStyle`
với style mặc định (bỏ tô đỏ), `Save`.

### 5. `App` cần biết đường dẫn Excel

Thêm field `excelPath string` vào `App` struct (`app.go:38`), set trong
`NewApp()` cùng lúc tạo `RealProcessor` (dùng chung giá trị
`resolveRepoFile("dondathang_test.xlsx")`).

Thêm method mới, theo đúng pattern `GetSTT`/`SetSTT` hiện có:

```go
// ConfirmPrice ghi đè giá của một dòng sản phẩm đã bị đánh dấu sai giá,
// theo lựa chọn của người dùng (giá PO hoặc giá hệ thống).
func (a *App) ConfirmPrice(row int, price float64) error {
	return excelwriter.ConfirmPrice(a.excelPath, row, price)
}
```

Wails tự sinh binding `wailsjs/go/main/App.ConfirmPrice` khi build/dev —
giống cách `GetSTT`/`ProcessFiles` đã có sẵn.

### 6. Frontend

- `types.ts`: thêm `PriceMismatchDetail` interface + field
  `priceMismatchDetails: PriceMismatchDetail[]` vào `OrderRow`.
- `ResultTable.tsx`: dòng nào có `priceMismatchCount > 0` thì badge
  "Đối soát giá" có thêm icon mở rộng (chevron), bấm vào (có
  `stopPropagation` để không kích hoạt click-to-copy của ô) sẽ toggle một
  `<tr>` phụ chèn ngay dưới, `colSpan` hết bảng, chứa bảng con: mỗi dòng
  là 1 `PriceMismatchDetail` — Mã | Tên SP | Giá PO | Giá hệ thống | 2
  nút "Dùng giá PO" / "Dùng giá hệ thống".
- Bấm 1 trong 2 nút → gọi `ConfirmPrice(detail.excelRow, price)` ngay
  (không hỏi xác nhận thêm, theo đúng yêu cầu). Thành công → cập nhật
  state cục bộ (Zustand) đánh dấu mã đó "đã chọn: giá PO/giá hệ thống",
  bấm lại nút còn lại vẫn gọi lại được (cho phép đổi ý). Lỗi → hiện dòng
  log lỗi qua `appendLog` (dùng lại cơ chế log đã có), không đổi state.
- Trạng thái "đã chọn cho mã nào" chỉ tồn tại trong phiên làm việc hiện
  tại (mất khi xử lý lại/tải lại app) — dữ liệu THẬT (giá trị ô Y trong
  Excel) đã được ghi thật, đây chỉ là hiển thị lại lựa chọn trong UI.

## Phạm vi

### Làm thật

- Field mới `PriceMismatchDetail`/`OrderRow.PriceMismatchDetails`.
- Đổi `WriteOrderRows` trả thêm `startRow` — 9 vendor + 3 test call site.
- Ghép chi tiết mismatch + tính `ExcelRow` — 9 vendor.
- `excelwriter.ConfirmPrice` + `App.ConfirmPrice`.
- Frontend: mở rộng dòng xem chi tiết, 2 nút áp dụng giá.

### Không làm (YAGNI, theo đúng lựa chọn của user)

- Không có nút "áp dụng cho cả đơn cùng lúc" — chỉ từng mã riêng lẻ.
- Không có dialog xác nhận trước khi ghi — bấm là ghi ngay.
- Không lưu trạng thái "đã chọn" xuyên suốt giữa các lần mở app/xử lý lại
  — chỉ trong phiên hiện tại.
- Không đổi luồng xử lý PDF hiện có (vẫn ghi Excel ngay lúc xử lý, mặc
  định dùng giá hệ thống cho mã sai giá) — tính năng này chỉ thêm bước
  XEM LẠI/SỬA LẠI sau đó.

## Rủi ro / lưu ý

- **`ConfirmPrice`'s an toàn dựa vào việc kiểm tra comment còn tồn tại**
  ở ô Y trước khi ghi đè — nếu người dùng mở file Excel thật bằng tay và
  xóa comment/sửa gì đó giữa lúc xử lý và lúc bấm nút, `ConfirmPrice` sẽ
  từ chối ghi thay vì âm thầm ghi sai — cần xác nhận hành vi
  `GetComments`/`DeleteComment` của excelize thật trong lúc viết code
  (Task 0 nên verify trực tiếp trên file test, không giả định).
- **Style "mặc định" để bỏ tô đỏ**: dự kiến dùng style ID 0 (mặc định
  của excelize) — cần verify thực tế nó không xóa mất định dạng khác của
  ô (ví dụ format số) đã có sẵn từ template gốc, không chỉ giả định.
- File Excel đang mở bằng Microsoft Excel thật khi bấm nút sẽ khiến
  `ConfirmPrice` lỗi ghi file — hành vi này giống hệt rủi ro đã tồn tại
  sẵn với `ProcessFiles`, không phải rủi ro mới, không cần xử lý đặc
  biệt thêm.
- Đổi signature `WriteOrderRows` là breaking change nội bộ — cần sửa
  ĐỒNG THỜI cả 9 vendor + test, không thể sửa từng phần (build sẽ đỏ nếu
  làm dở dang) — plan cần gộp việc này thành 1 task, không tách nhỏ theo
  vendor như các tính năng trước.

## Kiểm thử

- Test riêng cho `excelwriter.ConfirmPrice`: ghi đè giá thành công + xóa
  comment/tô đỏ; từ chối khi ô không có comment (dòng "chưa/không còn sai
  giá"); lỗi rõ ràng khi `row` ngoài phạm vi sheet.
- Test `WriteOrderRows` trả đúng `startRow` (đã có test hiện có, chỉ cần
  thêm assertion trên giá trị trả về mới).
- Với ít nhất 1 vendor thật (JMart, đã có sẵn kịch bản mismatch trong
  `TestRealProcessor_ProcessesRealSampleJMartFile`): assert
  `PriceMismatchDetails` có đúng số lượng, đúng `SKU`/giá, và `ExcelRow`
  trỏ đúng vào ô thật sự chứa comment/tô đỏ trong file đã ghi (đọc lại
  bằng `excelize.OpenFile` để verify, không chỉ tin vào con số tính
  toán).
- Không cần test riêng cho từng vendor còn lại ngoài build xanh + golden
  test hiện có vẫn pass (thay đổi là cộng thêm thuần túy, không đổi
  logic tính giá/khuyến mãi đã được test kỹ).
