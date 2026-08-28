# Fixture vàng nhánh TMĐT

Sinh lại bằng: `python docs/superpowers/plans/tmdt-golden-fixture.py`

| File | Nội dung |
|---|---|
| `orders.csv` | 1.585 dòng hàng, sheet "Đơn hàng haravan" của workbook thật, 22–23/08/2026 |
| `lookup.xlsx` | 2 bảng tra cứu lấy từ CHÍNH workbook đó (không lấy bản mới hơn — bảng tra cứu đổi theo thời gian, lấy lệch là golden sai) |
| `expected_dondathang.csv` | 1.430 dòng đầu ra đúng, sheet "Don dat hang" của `đơn hàng/mẫu chuẩn.xlsx`, RIÊNG cột V đã đổi mã kho (xem ghi chú cuối file) |
| `ten_hang.csv` | bản đồ MÃ TP → Tên hàng, dùng làm `ProductName` trong test |
| `expected_haravan_sheet.csv` | 1.585 dòng × 9 cột (`MÃ TP 1..4`, `SLTP1..4`, `Mã misa`) mà công thức Excel trong CHÍNH sheet "Đơn hàng haravan" tính ra — khoá phía sheet "Haravan", so theo VỊ TRÍ vì sheet giữ đúng thứ tự dòng hàng đầu vào |

Cột `S` (Tên hàng) KHÔNG có trong `expected_dondathang.csv`: nó tra
`productdata.Store` chứ không suy từ dữ liệu Haravan, nên so sánh nó ở
golden test là so kết quả với chính đầu vào. Việc `Build` có gọi đúng
`Options.ProductName` với mã thành phẩm hay không do `TestBuildQtyAndUnitPrice`
trong `mapping_test.go` khoá lại (nó kiểm `ProductName == "tên TP10127"`).

`expected_dondathang.csv` được so theo TẬP HỢP CÓ LẶP, không theo vị trí:
`mẫu chuẩn.xlsx` do người dùng dựng thủ công thành khối (ngày × mã MISA × kho),
còn `Build` giữ nguyên thứ tự dòng hàng đầu vào. `expected_haravan_sheet.csv`
thì so theo vị trí — xem chú thích trong `mapping_golden_test.go`.

## Cột V không còn khớp nguyên văn mẫu chuẩn

Ngày 27/08/2026 mã kho nhánh TMĐT đổi `TP_HN_12` → `TP_HN_13` và
`LA_KHOTMDT` → `LA_TP` (xem `warehouseOf` trong `internal/tmdt/mapping.go`).
`mẫu chuẩn.xlsx` do người dùng dựng TRƯỚC lần đổi đó nên vẫn mang mã cũ.

`expected_dondathang.csv` đã được đổi mã ở CHỈ cột V — 1.430 dòng, mỗi dòng
đúng một mã, không cột nào khác chứa hai chuỗi này. `tmdt-golden-fixture.py`
cũng đổi luôn khi sinh lại, nên chạy lại script không kéo golden về mã cũ.
Khi nào mẫu chuẩn được dựng lại bằng app thì xoá phép đổi ở cả hai nơi.
