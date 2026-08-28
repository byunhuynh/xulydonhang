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
# --- expected_haravan_sheet.csv: 9 cột mà công thức Excel tự tính ra trong
# CHÍNH sheet "Đơn hàng haravan" (BY..CF = MÃ TP 1..4 / SLTP1..4, CH = Mã misa).
# Đây là đầu ra mà tầng quy đổi phải sinh cho sheet "Haravan", nên nó khoá
# được phía sheet — thứ người dùng đọc — chứ không chỉ phía file hạch toán.
# Cột CG (Shop) bỏ vì orders.csv đã mang. Ghi ra ĐÚNG THỨ TỰ và ĐÚNG BỘ LỌC
# của orders.csv (cùng một vòng lặp) để test so được theo vị trí, dòng-với-dòng.
SHEET_COLS = [76, 77, 78, 79, 80, 81, 82, 83, 85]
SHEET_HEAD = ["tp1", "sl1", "tp2", "sl2", "tp3", "sl3", "tp4", "sl4", "misa"]
wb = openpyxl.load_workbook(MASTER, read_only=True, data_only=True)
ws = wb["Đơn hàng haravan"]
n = 0
with open(os.path.join(OUT, "orders.csv"), "w", newline="", encoding="utf-8") as fh,      open(os.path.join(OUT, "expected_haravan_sheet.csv"), "w", newline="", encoding="utf-8") as sh:
    w = csv.writer(fh)
    w.writerow(ORDER_HEAD)
    ws2 = csv.writer(sh)
    ws2.writerow(SHEET_HEAD)
    for row in ws.iter_rows(min_row=2, values_only=True):
        if not row or not row[0]:
            continue
        w.writerow([s(row[c]) if c < len(row) else "" for c in ORDER_COLS])
        # Ghi NGUYÊN VĂN ô trong workbook, kể cả ô dính ký tự xuống dòng ở cuối
        # (bảng "data shop" có một ô như vậy) — test tự TrimSpace phía fixture rồi
        # mới so, đúng cách code cắt khoảng trắng khi đọc bảng tra cứu.
        ws2.writerow([s(row[c]) if c < len(row) else "" for c in SHEET_COLS])
        n += 1
print("orders.csv:", n, "dòng")
print("expected_haravan_sheet.csv:", n, "dòng")

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
# Mã kho đổi ngày 27/08/2026 (warehouseOf trong internal/tmdt/mapping.go).
# "mẫu chuẩn.xlsx" là file người dùng dựng TRƯỚC lần đổi đó nên vẫn mang mã
# cũ; đổi tại đây để chạy lại script không lặng lẽ kéo golden về mã cũ và
# làm đỏ golden test. Bỏ hai dòng này đi khi mẫu chuẩn được dựng lại.
KHO_DOI_TEN = {"TP_HN_12": "TP_HN_13", "LA_KHOTMDT": "LA_TP"}
V = EXP_HEAD.index("V")
with open(os.path.join(OUT, "expected_dondathang.csv"), "w", newline="", encoding="utf-8") as fh:
    w = csv.writer(fh)
    w.writerow(EXP_HEAD)
    for row in gws.iter_rows(min_row=9, values_only=True):
        if not row or not row[0]:
            continue
        out = [s(row[c]) if c < len(row) else "" for c in EXP_COLS]
        out[V] = KHO_DOI_TEN.get(out[V], out[V])
        w.writerow(out)
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
