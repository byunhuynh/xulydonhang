# Fixture vàng nhánh TMĐT

Sinh lại bằng: `python docs/superpowers/plans/tmdt-golden-fixture.py`

| File | Nội dung |
|---|---|
| `orders.csv` | 1.585 dòng hàng, sheet "Đơn hàng haravan" của workbook thật, 22–23/08/2026 |
| `lookup.xlsx` | 2 bảng tra cứu lấy từ CHÍNH workbook đó (không lấy bản mới hơn — bảng tra cứu đổi theo thời gian, lấy lệch là golden sai) |
| `expected_dondathang.csv` | 1.430 dòng đầu ra đúng, sheet "Don dat hang" của `đơn hàng/mẫu chuẩn.xlsx` |
| `ten_hang.csv` | bản đồ MÃ TP → Tên hàng, dùng làm `ProductName` trong test |

Cột `S` (Tên hàng) KHÔNG có trong `expected_dondathang.csv`: nó tra
`productdata.Store` chứ không suy từ dữ liệu Haravan, nên so sánh nó ở
golden test là so kết quả với chính đầu vào. Xem `TestBuildGoiProductName`.
