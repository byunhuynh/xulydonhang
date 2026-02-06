import os
import shutil

# Thư mục nguồn và đích
src_dir = r"C:\Users\Admin\Desktop\code py\Xử lý đơn hàng\đơn hàng\10-2025"
dst_dir = r"C:\Users\Admin\Desktop\code py\Xử lý đơn hàng\đơn hàng"

# Danh sách file cần copy (không cần đuôi nếu tất cả là .pdf, .txt ... thì thêm vào)
file_names = """
96633797-00
96633793-00
96633754-00
96633796-00
96633798-00
96633794-00
96633792-00
96633799-00
96633800-00
96633801-00
96633795-00
96645588-00
96645584-00
96645587-00
96645513-00
96645585-00
96645583-00
96645586-00
96646538-00
96646522-00
96646534-00
96646527-00
96646541-00
96646520-00
96646514-00
96646510-00
96646537-00
96646530-00
96645653-00
96646525-00
96646516-00
96646543-00
96646512-00
96646526-00
96646535-00
96646540-00
96646542-00
96646532-00
96646521-00
96646513-00
96646519-00
96646515-00
96646511-00
96646517-00
96646529-00
96646533-00
96646539-00
96646524-00
96646528-00
96646531-00
96646518-00
96646523-00
96646509-00
96646536-00
96646986-00
96646982-00
96646985-00
96646987-00
96646983-00
96646981-00
96646988-00
96646696-00
96646984-00
""".strip().splitlines()

# Nếu file có đuôi, ví dụ .pdf thì thêm ở đây
EXT = ".pdf"

for name in file_names:
    src_file = os.path.join(src_dir, name + EXT)
    dst_file = os.path.join(dst_dir, name + EXT)
    if os.path.exists(src_file):
        shutil.copy2(src_file, dst_file)
        print(f"✅ Copied: {src_file} -> {dst_file}")
    else:
        print(f"⚠️ Không tìm thấy: {src_file}")
