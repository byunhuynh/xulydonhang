"""Sinh fixture vàng cho GO/internal/tmdt từ hai file Excel thật.

Chạy:  python docs/superpowers/plans/tmdt-golden-fixture.py

Đầu vào (đường dẫn tuyệt đối, máy người dùng):
  - master : XUẤT HÀNG HN-LA MỚI.xlsx  -> sheet "Đơn hàng haravan" + 2 bảng tra cứu
  - golden : đơn hàng/mẫu chuẩn.xlsx   -> sheet "Don dat hang", dữ liệu từ dòng 9
"""
import csv, os, openpyxl

# 4 lần dirname: bỏ tên file -> plans -> superpowers -> docs -> gốc worktree.
# (Công thức 3 lần dirname trong bản kế hoạch gốc chỉ lên tới "docs", thiếu
# một cấp — đã xác minh bằng cách chạy thử, script ghi nhầm vào docs/GO/...)
ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
MASTER = r"C:\Users\Admin\Desktop\Xuất hàng TMĐT\Tháng 07-2026\XUẤT HÀNG HN-LA MỚI.xlsx"
GOLDEN = os.path.join(ROOT, "đơn hàng", "mẫu chuẩn.xlsx")
OUT = os.path.join(ROOT, "GO", "internal", "tmdt", "testdata", "golden")
os.makedirs(OUT, exist_ok=True)

def s(v):
    return "" if v is None else str(v)

# --- orders.csv: 1 dòng cho mỗi dòng hàng trong sheet "Đơn hàng haravan" ---
ORDER_COLS = [0, 84, 69, 70, 16, 18, 19, 21, 26, 28]
ORDER_HEAD = ["order_code", "shop", "kho_ban", "kenh_ban_hang", "ngay_dat_hang",
              "so_luong", "ten_san_pham", "gia_tri_thuoc_tinh_1", "gia_san_pham", "ma_san_pham"]
wb = openpyxl.load_workbook(MASTER, read_only=True, data_only=True)
ws = wb["Đơn hàng haravan"]
n = 0
with open(os.path.join(OUT, "orders.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(ORDER_HEAD)
    for row in ws.iter_rows(min_row=2, values_only=True):
        if not row or not row[0]:
            continue
        w.writerow([s(row[c]) if c < len(row) else "" for c in ORDER_COLS])
        n += 1
print("orders.csv:", n, "dòng")

# --- lookup.xlsx: chỉ 2 bảng tra cứu, lấy từ CHÍNH master đã sinh golden ---
out_wb = openpyxl.Workbook()
out_wb.remove(out_wb.active)
# Chỉ sao tới DÒNG CÓ DỮ LIỆU CUỐI CÙNG của mỗi sheet. Sheet "data shop"
# của workbook gốc khai dimension tận A1:K1048576 (hơn một triệu phần tử
# <row>) trong khi chỉ ~291 dòng có dữ liệu. Sao nguyên khối thì lookup.xlsx
# phình lên 2,7 MB và excelize.GetRows phải quét cả triệu dòng rỗng mỗi lần
# golden test nạp bảng — chậm vô ích. Dòng trống không mang dữ liệu tra cứu
# nên cắt phần đuôi trống không đổi ngữ nghĩa fixture. Dòng trống Ở GIỮA
# vẫn phải giữ (sheet "Mã misa" có dòng 2 trống, vùng tra cứu bắt đầu từ dòng 3),
# nên đếm dồn rồi chỉ xả ra khi phía sau còn dữ liệu.
def _co_du_lieu(row):
    return any(c is not None and str(c).strip() != "" for c in row)

for name in ("Mã misa", "data shop"):
    src, dst = wb[name], out_wb.create_sheet(name)
    trong_cho = 0
    for row in src.iter_rows(values_only=True):
        if _co_du_lieu(row):
            for _ in range(trong_cho):
                dst.append([])
            trong_cho = 0
            dst.append(list(row))
        else:
            trong_cho += 1
out_wb.save(os.path.join(OUT, "lookup.xlsx"))
print("lookup.xlsx: đã sao 2 sheet tra cứu")

# --- expected_dondathang.csv: cột S bị loại, xem chú thích trong plan ---
EXP_COLS = [0, 1, 2, 3, 4, 6, 11, 16, 19, 20, 21, 23, 24, 30, 35, 38, 40, 47]
EXP_HEAD = ["A", "B", "C", "D", "E", "G", "L", "Q", "T", "U", "V", "X", "Y", "AE", "AJ", "AM", "AO", "AV"]
gwb = openpyxl.load_workbook(GOLDEN, read_only=True, data_only=True)
gws = gwb["Don dat hang"]
m = 0
with open(os.path.join(OUT, "expected_dondathang.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(EXP_HEAD)
    for row in gws.iter_rows(min_row=9, values_only=True):
        if not row or not row[0]:
            continue
        w.writerow([s(row[c]) if c < len(row) else "" for c in EXP_COLS])
        m += 1
print("expected_dondathang.csv:", m, "dòng")

# --- ten_hang.csv: bản đồ MÃ TP -> Tên hàng, dùng làm ProductNamer trong test ---
names = {}
for row in gws.iter_rows(min_row=9, values_only=True):
    if row and row[0] and row[16]:
        names.setdefault(s(row[16]), s(row[18]))
with open(os.path.join(OUT, "ten_hang.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(["ma_tp", "ten_hang"])
    for k in sorted(names):
        w.writerow([k, names[k]])
print("ten_hang.csv:", len(names), "mã")
