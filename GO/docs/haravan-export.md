# haravan-order-export

> **Cập nhật 25/08/2026:** logic của tool này đã được đưa vào app chính
> thành nhánh xử lý TMĐT — thả `XUẤT HÀNG HN-LA MỚI.xlsx` vào app rồi bấm
> "Xử lý". Xem `docs/superpowers/specs/2026-08-25-tmdt-haravan-branch-design.md`.
> CLI ở `GO/cmd/haravan-export` vẫn giữ để đối chiếu và cho hai bố cục
> `haravan` / `full` mà app không dùng.

Lấy đơn hàng từ **Haravan Omni API** (đơn đồng bộ về từ **Shopee** và **TikTok Shop**) rồi ghi ra file **Excel**.

## 1. Access token

Haravan xác thực bằng Bearer token: `Authorization: Bearer <access_token>`.

Cách nhanh nhất là **Private token**: vào https://partners.haravan.com → App của bạn → **Private Token** → **Create Private Token**, tick quyền **`com.read_orders`**.

Đặt token vào file `.env` ở thư mục gốc dự án (file này đã được `.gitignore`):

```
HARAVAN_ACCESS_TOKEN=xxxxxxxxxxxxxxxxxxxx
```

Tool tự nạp `.env` khi khởi động. Hoặc dùng biến môi trường / cờ `-token`.

> Đừng để token trong `.env.example` — file đó là mẫu và sẽ bị commit.

## 2. Chạy

```powershell
go build -o haravan-export.exe ./cmd/haravan-export

# file hoàn chỉnh, đã tính sẵn MÃ TP / SLTP / Shop / Mã misa
./haravan-export.exe -from 2026-08-24 -to 2026-08-25 -out "XUẤT HÀNG 25-08 HN-LA MỚI.xlsx" -mapping "C:\Users\Admin\Desktop\Xuất hàng TMĐT\Tháng 07-2026\XUẤT HÀNG HN-LA MỚI.xlsx"

# thử nhanh 100 đơn
./haravan-export.exe -out thu.xlsx -max 100 -mapping "...\XUẤT HÀNG HN-LA MỚI.xlsx"
```

`-mapping` trỏ tới workbook chứa hai sheet tra cứu `data shop` và `Mã misa`. Mặc định tìm `XUẤT HÀNG HN-LA MỚI.xlsx` trong thư mục hiện tại.

Đơn của shop **CLEVY VIỆT NAM** bị loại sẵn (`-exclude-shop`). Muốn lấy hết thì truyền `-exclude-shop ""`, muốn loại thêm shop khác thì liệt kê ngăn bởi dấu phẩy.

## 3. Ba bố cục file

### `-format chuan` (mặc định) — file hoàn chỉnh, KHÔNG còn công thức

Đúng 23 cột thực sự dùng, xếp liền nhau: 12 cột dữ liệu Haravan + 11 cột trước đây do công thức Excel sinh ra, nay Go **tính sẵn**.

| Cột | Tiêu đề | Lấy từ đâu |
|---|---|---|
| A | Mã đơn hàng | `name` |
| B | Tổng tiền | `subtotal_price` |
| C | Tổng cộng | `total_price` |
| D | Ngày đặt hàng | `created_at` → chuỗi ISO `+07:00` |
| E | Số lượng sản phẩm | `line_items[].quantity` |
| F | Tên sản phẩm | `line_items[].title` |
| G | Giá trị thuộc tính 1 | `line_items[].variant_title` |
| H | Giá sản phẩm | `line_items[].price` |
| I | Mã sản phẩm | `line_items[].sku` |
| J | Thuộc tính | `note_attributes` dạng `Tên : Giá trị` |
| K | Kho bán | `location_name` |
| L | Kênh bán hàng | `source_name` |
| M | Thời gian Đặt | `created_at` → giờ VN, **ngày thật** (hiển thị `mm-dd-yy`) |
| N–U | MÃ TP 1..4 / SLTP1..4 | Tra sheet `data shop`: **có** Mã sản phẩm → tra theo `Mã combo`; **không có** → tra theo `Tên sản phẩm + Phân loại` (khớp các dòng bỏ trống Mã combo). Giá trị 0 → ô trống |
| V | Shop | `note_attributes["X-Haravan-SalesChannel-BranchName"]` — đọc thẳng, không phải cắt chuỗi |
| W | Mã misa | Tra sheet `Mã misa` theo tên shop (không phân biệt hoa thường) |

Không tìm thấy trong bảng tra cứu thì ghi `#N/A` **và in cảnh báo ra màn hình** kèm số dòng, để bạn biết cần bổ sung gì:

```
CẢNH BÁO — 1 mục chưa khai báo trong bảng tra cứu:
  - shop "XYZ" chưa có trong sheet "Mã misa" (12 dòng → Mã misa = #N/A)
```

Hai bảng tra cứu vẫn đọc từ workbook của bạn (cờ `-mapping`) vì đó là nơi bạn thêm sản phẩm mới — code chỉ đọc, không sửa.

**Đã đối chiếu với file thật:** xuất lại đúng khoảng 22–23/08/2026 rồi so từng ô với `XUẤT HÀNG HN-LA MỚI.xlsx` → **1.564/1.564 đơn, 1.585 dòng, trùng khớp 100%** trên cả 12 cột dữ liệu lẫn 11 cột trước đây dùng công thức.

### `-format haravan` — thô như Haravan xuất ra

Giống hệt file Excel mà trang quản trị Haravan xuất ra: sheet **"Đơn hàng haravan"**, 75 cột `A..BW` với đúng tiêu đề gốc, **một dòng cho mỗi dòng hàng**. Tool chỉ điền 12 cột được yêu cầu, phần còn lại để trống:

| Cột | Tiêu đề | Lấy từ API |
|---|---|---|
| A | Mã đơn hàng | `name` |
| J | Tổng tiền | `subtotal_price` |
| M | Tổng cộng | `total_price` |
| Q | Ngày đặt hàng | `created_at` → chuỗi ISO `+07:00` |
| S | Số lượng sản phẩm | `line_items[].quantity` |
| T | Tên sản phẩm | `line_items[].title` |
| V | Giá trị thuộc tính 1 | `line_items[].variant_title` |
| AA | Giá sản phẩm | `line_items[].price` |
| AC | Mã sản phẩm | `line_items[].sku` |
| BB | Thuộc tính | `note_attributes` dạng `Tên : Giá trị`, ngăn bằng xuống dòng |
| BR | Kho bán | `location_name` |
| BS | Kênh bán hàng | `source_name` |

Bố cục này chỉ có dữ liệu thô từ Haravan, không có cột tính sẵn — dùng khi bạn muốn tự xử lý tiếp. Vị trí 75 cột được giữ nguyên để tương thích với công thức tham chiếu theo chữ cái cột.

### `-format full`

Ba sheet tự thiết kế, nhiều thông tin hơn:

| Sheet | Nội dung |
|---|---|
| `DonHang` | 1 dòng / đơn: sàn, tên shop, mã đơn, ngày đặt hàng, trạng thái, khách hàng, địa chỉ giao, tiền hàng / giảm giá / phí vận chuyển / tổng tiền, dịch vụ + mã vận đơn, kho |
| `ChiTietSanPham` | 1 dòng / sản phẩm: tên sản phẩm, SKU, barcode, thuộc tính, số lượng, giá bán, giá gốc, thành tiền |
| `TongHop` | Số đơn / số sản phẩm / doanh thu theo **từng shop** của từng sàn |

Có bộ lọc trên dòng tiêu đề và freeze dòng đầu; tiền định dạng `#,##0`, ngày giờ theo **giờ Việt Nam (GMT+7)**.

## 4. Nhận diện đơn Shopee / TikTok Shop

Tài liệu Omni API **không có** trường "sàn" riêng. Trường gần nhất là
[`source_name`](https://docs.haravan.com/docs/omni-apis/orders/) — *"where the order originated"* — và Haravan chỉ bảo lưu 5 giá trị `web`, `pos`, `haravan_draft_order`, `iphone`, `android`; giá trị khác do app tạo đơn tự đặt. Nghĩa là **dấu vết sàn nằm ở đâu là tuỳ cấu hình kết nối của từng shop**.

Vì vậy tool dò từ khoá trên **tất cả** các trường có thể chứa nó — `source_name`, `tags`, `utm_*`, `landing_site`, `ref_order_number`, `note`, `gateway`, `note_attributes`, `fulfillments[].tracking_company` — xem [`internal/haravan/channel.go`](internal/haravan/channel.go).

Từ khoá mặc định: Shopee ← `shopee`, `shoppe`, `spx` — TikTok Shop ← `tiktok`, `tik tok`, `tiktokshop`, `tts`.

Muốn kiểm tra store của bạn gắn nhãn thế nào, chạy chế độ dò (không ghi file):

```powershell
./haravan-export.exe -discover -max 200
```

rồi chỉnh lại bộ từ khoá nếu cần:

```powershell
./haravan-export.exe -keywords "Shopee=shopee,spx;TikTok Shop=tiktok,tts"
```

### Tên shop nằm ở đâu

Haravan Omnichannel gắn shop nguồn vào `note_attributes`:

| Khoá | Ý nghĩa | Cột Excel |
|---|---|---|
| `X-Haravan-SalesChannel-BranchName` | Tên shop trên sàn | **Tên shop** |
| `X-Haravan-SalesChannel-BranchId` | ID shop bên sàn | ID shop sàn |

Kiểm tra trên 400 đơn của store này: **100% đơn đều có hai khoá trên**. Xử lý ở [`haravan.ShopName` / `ShopID`](internal/haravan/channel.go).

Các trường khác cho những giá trị bạn cần:

| Giá trị cần | Nguồn trong API |
|---|---|
| Tên sản phẩm | `line_items[].title` |
| Mã sản phẩm | `line_items[].sku` (trùng `barcode`) |
| Tên thuộc tính | `line_items[].variant_title`, cộng thêm `line_items[].properties` nếu có (đã lọc bỏ khoá nội bộ `X-Haravan-*`) |
| Ngày đặt hàng | `created_at`, đổi sang GMT+7 |
| Giá gốc / giá so sánh | `line_items[].price_original` |
| Phí + dịch vụ vận chuyển | `shipping_lines[].price` / `.title` |
| Kho xử lý đơn | `location_name` |

### Ghi nhận từ lần chạy thật trên store này

- `source_name` rất sạch: **`tiktokshop`** và **`shopee`** → bộ từ khoá mặc định khớp 100%. (Đơn còn có trường `source` không nằm trong tài liệu, giá trị giống `source_name`.)
- Store đang bán qua **6 shop**: TikTok — *Tẩy lồng máy giặt Blue*, *CLEVY VIỆT NAM*, *Nước Giặt Xả Blue*; Shopee — *Blue Việt Nam*, *BLUE HN*, *Be Clean Việt Nam*.
- `tags` chỉ chứa dịch vụ giao hàng (`Next-day delivery`, `Standard shipping`, `Nhanh`), không dùng để nhận diện sàn.
- Haravan đặt **mã đơn của sàn thẳng vào `order.name`** (ví dụ `585206619912505206`, `260726MBH1SPAY`), `ref_order_number` để trống → cột **Mã đơn** chính là mã tra cứu trên Shopee/TikTok. Cột "Mã đơn trên sàn" chỉ có giá trị khi store lưu mã ở trường riêng.
- Đơn từ sàn bị **che dữ liệu khách** ở phía sàn: tên dạng `N******n`, email `guest@haravan.com`, SĐT trống, địa chỉ che một phần. Đây là dữ liệu sàn trả về, không phải lỗi tool.
- `product_id` / `variant_id` = null với đơn sàn; định danh sản phẩm dùng **SKU**. Một số dòng hàng sàn không gửi SKU nên cột đó trống.
- `line_items[].properties` với đơn sàn chỉ chứa khoá nội bộ `X-Haravan-SalesChannel-LineId` — đã lọc bỏ khỏi cột Thuộc tính.

## 5. Tuỳ chọn

```
-token                 Access token (mặc định lấy từ .env / HARAVAN_ACCESS_TOKEN)
-from / -to            Khoảng ngày YYYY-MM-DD, giờ VN (mặc định 30 ngày gần nhất)
-date-field            created | updated — lọc theo created_at hay updated_at
-status                open | closed | cancelled | any   (mặc định any)
-financial-status      paid | pending | refunded | ...
-fulfillment-status    shipped | unshipped | partial
-channels              shopee,tiktok | all   (mặc định shopee,tiktok)
-keywords              Ghi đè bộ từ khoá: "Shopee=shopee;TikTok Shop=tiktok"
-include-other         Xuất luôn đơn không nhận diện được sàn (gắn nhãn "Khác")
-exclude-shop          Bỏ qua đơn của các shop này (mặc định "CLEVY VIỆT NAM"); để rỗng nếu muốn lấy hết
-format                chuan (mặc định, đã tính sẵn) | haravan (thô) | full (3 sheet)
-mapping               Workbook chứa sheet "data shop" và "Mã misa" (cho -format chuan)
-out                   File Excel đầu ra
-max                   Giới hạn số đơn tải về (0 = không giới hạn)
-discover              Chỉ liệt kê source_name / tags, không ghi Excel
```

## 6. Ghi chú kỹ thuật

- Endpoint: `GET https://apis.haravan.com/com/orders.json` và `/com/orders/count.json`.
- Phân trang `page` + `limit` (**tối đa 50/trang**), sắp xếp `created_at asc`; dừng khi trang trả về ít hơn `limit`.
- **Lọc theo ngày:** dùng `created_at_min` / `created_at_max` (hoặc `updated_at_*`). Lọc theo từng ngày là chính xác tuyệt đối — kiểm chứng ngày 23/24/25-08-2026: 797 + 761 + 317 = 1.875, đúng bằng số đơn của cửa sổ 23→25. Đơn sớm nhất/muộn nhất của ngày 24/08 là `00:04:15` và `23:54:28` giờ VN.
- **Bẫy múi giờ:** Haravan so các mốc này theo **giờ cửa hàng (GMT+7)**, và xử lý hậu tố múi giờ rất lạ: `...T00:00:00` → 761 đơn (đúng), `...T00:00:00Z` → 761 đơn (chữ `Z` bị bỏ qua), nhưng `...T00:00:00+07:00` → **786 đơn** (offset số bị quy đổi về UTC trước, rồi giờ UTC đó mới đem so như giờ VN ⇒ lệch 7 tiếng). Phải gửi giờ VN dạng "trần"; bản đầu của tool đổi sang UTC rồi mới format nên **mất 267 đơn và lấy thừa 242 đơn** mà không báo lỗi gì. Đã khoá lại bằng test `TestListOptionsSendsShopLocalTime`.
- Rate limit: leaky bucket **80 request, rò 4 req/s**. Client đọc header `X-Haravan-Api-Call-Limit` và tự nghỉ khi dùng quá 80% bucket; gặp `429` thì chờ theo `Retry-After` rồi thử lại (backoff luỹ thừa, tối đa 5 lần). Lỗi `401/403` **không** retry — báo lỗi ngay để kiểm tra token/scope.
- **Ghi streaming**: đơn được đẩy thẳng vào file qua `StreamWriter` của excelize ngay khi từng trang API về, không gom hết vào RAM. Cần thiết vì store này có ~21.600 đơn / 30 ngày (~433 trang API).
- Số tiền trong response Haravan lúc là number lúc là string → kiểu `haravan.Number` xử lý được cả hai.

## 7. Kiểm thử

```powershell
go test ./...
```

Test dùng `httptest` giả lập Haravan (phân trang, 429/`Retry-After`, 401, JSON kiểu hỗn hợp) và mở lại file `.xlsx` để kiểm tra nội dung — không cần token thật.

## Nguồn tài liệu

- [Haravan Omni APIs](https://docs.haravan.com/docs/omni-apis/)
- [Order API](https://docs.haravan.com/docs/omni-apis/orders/)
- [API call limit](https://docs.haravan.com/docs/omni-apis/api-call-limit/)
- [Private app authentication](https://docs.haravan.com/docs/tutorials/authentication/private-app-authentication/)
