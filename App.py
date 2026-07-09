import sys
import os
from PySide6.QtWidgets import QApplication, QMainWindow, QListView
from PySide6.QtCore import Qt, QStringListModel
from form_ui import Ui_MainWindow  # Import giao diện từ file UI
from PySide6.QtCore import QThread, Signal
import xulydonhang
from PySide6.QtWidgets import  QTableWidgetItem, QHeaderView
import pygetwindow as gw
import pyautogui
import time
import psutil
from PySide6.QtGui import QColor, QFont
import requests
import re
from datetime import datetime
import json
import send_zalo

url = "https://script.google.com/macros/s/AKfycbyeg7Mu72mWOIEv5gYDsiGSY9FB7d-OiPdzM-PdAdBapTp4sUhb7ly1_N2ZtvqUrW8wBA/exec"
FOLDER_PATH = os.path.abspath("đơn hàng")  # Chuyển "đơn hàng" sang đường dẫn tuyệt đối
ALLOWED_EXTENSIONS = {".pdf", ".xlsx", ".txt"}  # Chỉ nhận các file có định dạng này


window_title = "Blue Hà Thành - Order System v2.0"
def bring_window_to_top(window_title):
    """Đưa cửa sổ có tiêu đề window_title lên top"""
    windows = gw.getWindowsWithTitle(window_title)
    if windows:
        win = windows[0]
        if win.isMinimized:
            win.restore()  # Khôi phục nếu bị thu nhỏ
        win.activate()  # Đưa cửa sổ lên trước
        time.sleep(0.5)  # Chờ một chút để đảm bảo nó lên top
        pyautogui.click(win.left + 10, win.top + 10)  # Click để đảm bảo focus
        return True
    return False
def is_already_running():
    """Kiểm tra xem chương trình đã chạy chưa"""
    current_pid = os.getpid()
    current_name = os.path.basename(sys.argv[0])

    for process in psutil.process_iter(['pid', 'name']):
        if process.info['name'] == current_name and process.info['pid'] != current_pid:
            return True
    return False

class FileProcessorThread(QThread):
    log_signal = Signal(str)  # Tín hiệu gửi log
    finished_all = Signal()  # Tín hiệu khi hoàn thành tất cả file
    table_signal = Signal(str, str, str, str,str, str, str)  # Thêm tín hiệu gửi dữ liệu bảng
    stt_signal = Signal(int)  # 🚀 Signal mới để gửi STT về giao diện
    

    def __init__(self, file_list,stt,table_widget,pushMisa):
        super().__init__()
        self.file_list = file_list
        self.stt = stt  # Lưu STT để sử dụng
        self.table_widget = table_widget   # ← Lưu widget bảng vào đây
        self.pushMisa = pushMisa
        
        xulydonhang.process_handler.log_signal.connect(self.send_log)  # Nhận log từ xulydonhang
        xulydonhang.process_handler.table_signal.connect(self.send_table)  # Nhận tín hiệu cập nhật bảng

        
         # Đặt chính sách kích thước tự động mở rộng
    def send_table(self, filename, page, system,makhachhang, po, saigia, status):
        """Nhận dữ liệu bảng từ xulydonhang và gửi về giao diện"""
        self.table_signal.emit(filename, page, system,makhachhang, po, saigia, status)
        

   
    def send_log(self, message):
        """Nhận log từ xulydonhang và gửi về giao diện"""
        self.log_signal.emit(message)
        


    def run(self):
        """Chạy lần lượt từng file"""
        for file in self.file_list:
            try:
                xulydonhang.process_handler.process_file(file, self.stt)  # Truyền STT vào xử lý
            except Exception as e:
                self.log_signal.emit(f"❌ Lỗi khi xử lý file <b>{file}</b>: {e}")
            kiemtragia = 0

            self.stt = self.stt + 1
            self.stt_signal.emit(self.stt)  # 🚀 Gửi STT mới về giao diện
            
                
        self.finished_all.emit()


class ZaloSenderThread(QThread):
    log_signal = Signal(str)       # Log từng dòng, gửi ngay khi có (không đợi xong hết)
    item_signal = Signal(dict)     # Kết quả từng nhóm tin vừa xử lý xong
    status_signal = Signal(int, int)  # (current, total) tiến độ gửi tin
    error_signal = Signal(str)     # Lỗi tổng quát (vd. không mở được Chrome)
    finished_all = Signal()

    def run(self):
        try:
            send_zalo.gui_tinnhan(
                on_progress=self.item_signal.emit,
                on_status=self.status_signal.emit,
            )
        except Exception as e:
            self.error_signal.emit(str(e))
        finally:
            self.finished_all.emit()


class MyApp(QMainWindow):
    def __init__(self):
        super().__init__()
        self.ui = Ui_MainWindow()
        self.ui.setupUi(self)

        # Tạo model để lưu danh sách file
        self.model = QStringListModel()
        self.file_list = []  # Lưu danh sách file dưới dạng đường dẫn đầy đủ
        self.ui.listdanhsach.setModel(self.model)
        self.ui.listdanhsach.setSelectionMode(QListView.ExtendedSelection)  # Chọn nhiều file
        self.ui.tableStatus.setColumnCount(7)  # 2 cột: Tên file & Trạng thái
        self.ui.tableStatus.setHorizontalHeaderLabels(["Tên file","Trang","Hệ thống","Mã khách hàng","PO","Đơn giá", "Trạng thái"])
        self.ui.traHang.setColumnCount(3)
        self.ui.traHang.setHorizontalHeaderLabels(["Tên file","PO", "Trả hàng"])
        




        #self.ui.groupBox_6.setHidden(True)
        #self.ui.groupBox_5.setGeometry(10, 259, 531, 220)
        #self.ui.tableStatus.setGeometry(10, 20, 511, 190)
        STATE_FILE = "zalo_state.json"

        if not os.path.exists(STATE_FILE):
            self.ui.zalo_btn.hide()   # Ẩn nút
        else:
            self.ui.zalo_btn.show()  # (không bắt buộc, nhưng rõ ràng)
            

        #groupBox_6
        self.ui.groupBox_6.setHidden(True)
        self.ui.pushMisa.setHidden(True)
        # Gọi hàm tạo thư mục tháng-năm ngay khi khởi động
        self.current_order_folder = self.ensure_monthly_order_folder()
        

        # Kích hoạt kéo thả file
        self.ui.listdanhsach.setAcceptDrops(True)
        self.setAcceptDrops(True)

        # Load file khi mở ứng dụng
        self.load_files_from_folder()
        stt = xulydonhang.lay_gia_tri_G1() +1 
        ghiheader = xulydonhang.write_headers()
        if ghiheader:
            self.ui.log.append(f"⚠️ File {ghiheader} đang mở trong Excel. Hãy đóng nó rồi chạy lại.")


        self.ui.stt.setText(str(stt))  # Chuyển giá trị sang chuỗi trước khi đặt vào QLabel

        # Xử lý sự kiện
        self.ui.xacnhan.clicked.connect(self.xu_ly_don_hang)  # Xử lý khi nhấn nút
        self.ui.loadfile.clicked.connect(self.xoanhatky)  # Xử lý khi nhấn nút
        self.ui.loadfile.clicked.connect(self.load_files_from_folder)  # Xử lý khi nhấn nút
        self.ui.zalo_btn.clicked.connect(self.gui_zalo)
        

        

        self.ui.listdanhsach.keyPressEvent = self.xoa_file  # Nhấn Delete để xóa file khỏi danh sách
       
        active, message = self.check_lock("https://script.googleusercontent.com/macros/echo?user_content_key=AehSKLgV3aPK8-f8VNJwzP90ajqVHPrVA79EFXzEXNYwrl9a54FprnPC1zi38wV1DWN784FxkOHL4WsuQDLudGj0fIXRb9OuiA_fPs4ywbl7c0HTNt1SE2tpqk6RBRfJqv0xZh4N9LVKeKO6EB7SZS3cIWMyIfzzFKpgEArXcuzS-Uy0lmzhpAr5lKU-Ty7m8LNEyNW9a5Z_IN464LDfEpQLEMpvt7cpiS8dxCxSAz_d778KpxTAnNj5uQtViioM2bAtD9bfrWbfMf7Xod-idEFvoEKouvTLTcqBLmNJetxr&lib=MqsGBAay818OJ-NUt2Er7nVZX_pgFzBjH")
         

        # ❌ KHÓA APP nếu KHÔNG active
        if active != 1:
            self.ui.log.clear()

            for line in message.split('\n'):
                self.ui.log.append(line)

            self.setEnabled(False)  # khóa toàn bộ app
            return




        '''

        if noidunglock:
            if noidunglock == "Không kết nối":
                self.ui.log.append(f'Không kết nối được tới internet')
                return
            else:
                lock = re.search(r"<lock>(.*?)</lock>", noidunglock).group(1)
                message_match = re.search(r"<message>(.*?)</message>", noidunglock, re.DOTALL)
                message = message_match.group(1).strip()  # .strip() để bỏ khoảng trắng đầu/cuối
                if lock == '1':
                    self.ui.log.clear()
                    for line in message.split('\n'):
                        self.ui.log.append(line)
                    self.setEnabled(False)  # ✅ Đúng

'''



    

    
    # ---- Gọi thử ----
    # write_headers("dondathang.xlsx", "Don dat hang", 7)




    def _lock_ui_for_processing(self, locked: bool):
        """
        Khóa/mở khóa các nút thao tác chính trong khi đang xử lý đơn hàng HOẶC đang
        gửi Zalo. Cả hai luồng đều đọc/ghi message.txt và dondathang.xlsx, nên chạy
        chồng lên nhau (ví dụ bấm "Xác nhận" trong lúc đang gửi Zalo) sẽ gây lỗi.
        """
        enabled = not locked
        self.ui.xacnhan.setEnabled(enabled)
        self.ui.loadfile.setEnabled(enabled)
        self.ui.listdanhsach.setEnabled(enabled)
        self.ui.stt.setEnabled(enabled)
        self.ui.zalo_btn.setEnabled(enabled)

    def gui_zalo(self):
        self._lock_ui_for_processing(True)
        self.ui.log.append("⏳ Đang gửi Zalo...")
        self._zalo_had_error = False

        self.zalo_thread = ZaloSenderThread()
        self.zalo_thread.item_signal.connect(self.on_zalo_item)
        self.zalo_thread.status_signal.connect(self.on_zalo_status)
        self.zalo_thread.error_signal.connect(self.on_zalo_error)
        self.zalo_thread.finished_all.connect(self.on_zalo_finished)
        self.zalo_thread.start()

    def on_zalo_status(self, current, total):
        """Báo tiến độ gửi tin để người dùng biết đang gửi tin thứ mấy mà chờ."""
        if total == 0:
            self.ui.log.append("ℹ️ Không có tin nhắn nào cần gửi.")
        elif current == 0:
            self.ui.log.append(f"🔔 Có {total} tin nhắn cần gửi.")
        else:
            self.ui.log.append(f"📨 Đang gửi tin nhắn thứ {current}/{total}...")

    def on_zalo_item(self, item):
        """Cập nhật nhật ký ngay khi 1 nhóm tin vừa được xử lý xong, thay vì đợi gửi hết mới hiển thị."""
        self.ui.log.append(item["zalo"] or "KHÔNG CÓ NHÓM")
        self.ui.log.append("nội dung:")
        self.ui.log.append(item["message"])
        self.ui.log.append(f"trạng thái: {item['status']}")
        self.ui.log.append("-----")

    def on_zalo_error(self, message):
        self._zalo_had_error = True
        self.ui.log.append(f"❌ Lỗi gửi Zalo: {message}")

    def on_zalo_finished(self):
        if not self._zalo_had_error:
            self.ui.log.append("✅ Đã gửi Zalo thành công")
        self._lock_ui_for_processing(False)

    def ensure_monthly_order_folder(self):
        """
        Đảm bảo tồn tại:
          - Thư mục gốc 'đơn hàng'
          - Thư mục con 'MM-YYYY' cho tháng và năm hiện tại
        Trả về đường dẫn tuyệt đối đến thư mục 'MM-YYYY'.
        """
        # 1. Thư mục gốc
        base_folder = os.path.abspath("đơn hàng")
        if not os.path.exists(base_folder):
            os.makedirs(base_folder)
            self.ui.log.append(f"Đã tạo thư mục gốc '{base_folder}'.")
        
        # 2. Thư mục tháng-năm hiện tại
        month_year = datetime.now().strftime("%m-%Y")  # ví dụ "04-2025"
        monthly_folder = os.path.join(base_folder, month_year)
        if not os.path.exists(monthly_folder):
            os.makedirs(monthly_folder)
            self.ui.log.append(f"Đã tạo thư mục '{monthly_folder}'.")
        
        return monthly_folder
                    


        


    def export_table_to_log(self):
        """Xuất toàn bộ nội dung tableStatus ra file log.log (append mode), với cột căn đều."""
        # 1. Thu thập dữ liệu (bao gồm header và tất cả các row)
        row_count = self.ui.tableStatus.rowCount()
        col_count = self.ui.tableStatus.columnCount()

        # Header
        headers = [
            (self.ui.tableStatus.horizontalHeaderItem(col).text()
             if self.ui.tableStatus.horizontalHeaderItem(col) else '')
            for col in range(col_count)
        ]
        # Các dòng dữ liệu
        rows = []
        for row in range(row_count):
            row_vals = []
            for col in range(col_count):
                item = self.ui.tableStatus.item(row, col)
                row_vals.append(item.text() if item else '')
            rows.append(row_vals)

        # 2. Tính chiều rộng tối đa cho mỗi cột
        all_rows = [headers] + rows
        col_widths = [
            max(len(r[col]) for r in all_rows)
            for col in range(col_count)
        ]

        # 3. Ghi file với padding
       
        # Ví dụ `headers`, `rows`, `col_count`, `col_widths` đã có sẵn từ phần trước
        with open('log.log', 'a', encoding='utf-8') as f:
            f.write(f"\n----- BẢNG STATUS lúc {datetime.now().strftime('%H:%M:%S %d/%m/%Y')} -----\n")

            # Header
            line = ' | '.join(headers[col].ljust(col_widths[col]) for col in range(col_count))
            f.write(line + '\n')

            # Ghi từng dòng
            for row_vals in rows:
                line = ' | '.join(row_vals[col].ljust(col_widths[col]) for col in range(col_count))
                f.write(line + '\n')

            f.write("----- Kết thúc bảng -----\n")

            # ⚡ Tạo payload để gửi
            data_list = []
            for row_vals in rows:
                item = {headers[col]: row_vals[col] for col in range(col_count)}
                data_list.append(item)

            payload = {
                "dulieu": data_list,
                "thoigian": datetime.now().strftime('%H:%M:%S %d/%m/%Y')
            }

            # Gửi POST
            url = "https://script.google.com/macros/s/AKfycbyeg7Mu72mWOIEv5gYDsiGSY9FB7d-OiPdzM-PdAdBapTp4sUhb7ly1_N2ZtvqUrW8wBA/exec"
            response = None
            response_data = None
            try:
                response = requests.post(url, data=json.dumps(payload), headers={'Content-Type': 'application/json'}, timeout=15)
                response.raise_for_status()
                response_data = response.json()
            except requests.exceptions.RequestException as e:
                print("⚠️ Lỗi gửi dữ liệu lên Google Sheets:", e)
                f.write(f"⚠️ Lỗi gửi dữ liệu lên Google Sheets: {e}\n")
            except json.JSONDecodeError:
                raw_text = response.text if response is not None else ''
                print("⚠️ Phản hồi từ Google Sheets không phải JSON hợp lệ:", raw_text)
                f.write(f"⚠️ Phản hồi từ Google Sheets không phải JSON hợp lệ: {raw_text}\n")

            duplicated_pos = response_data.get("duplicatedPOs", []) if response_data else []

            print("📥 duplicated_pos =", duplicated_pos)

            # ⚙️ Lấy số dòng và cột trong tableStatus
            row_count = self.ui.tableStatus.rowCount()
            col_count = self.ui.tableStatus.columnCount()

            print(f"🔍 Có {row_count} dòng, {col_count} cột trong tableStatus")

            # 🔁 Duyệt từng dòng trong bảng
            for row in range(row_count):
                po_item = self.ui.tableStatus.item(row, 4)  # Cột PO (index 5)

                if not po_item:
                    print(f"❌ Dòng {row}: không có item ở cột PO")
                    continue

                po_text = po_item.text().strip()
                print(f"Row {row} - PO: '{po_text}'")

                if po_text in duplicated_pos:
                    print(f"🚨 Dòng {row} có PO trùng: '{po_text}'")

                    # ✅ Ghi "Trùng lặp" vào cột cuối
                    note_item = QTableWidgetItem("Trùng lặp")
                    note_item.setForeground(QColor("red"))
                    font = QFont()
                    font.setBold(True)
                    note_item.setFont(font)
                    self.ui.tableStatus.setItem(row, col_count - 1, note_item)

                    # 🎨 Tô màu nền dòng
                    for col in range(col_count):
                        cell = self.ui.tableStatus.item(row, col)
                        if not cell:
                            print(f"⚠️ Dòng {row}, cột {col} chưa có item – tạo mới")
                            cell = QTableWidgetItem("")
                            self.ui.tableStatus.setItem(row, col, cell)

                        cell.setBackground(QColor("#FFD6D6"))  # Hồng nhạt
                else:
                    print(f"✅ Dòng {row} PO không trùng: '{po_text}'")

            if response is not None:
                print("Status Code:", response.status_code)
                print("Response:", response.text)

        if response is not None:
            self.ui.log.append(response.text)
        self.ui.log.append("✅ Đã xuất tableStatus vào log.log (cột đã căn đều)")

        





    def check_lock(self, url: str):
        """
        Chỉ cho phép chạy khi active == 1
        Mọi lỗi khác → coi như bị khóa
        """
        try:
            resp = requests.get(url, timeout=5)
            resp.raise_for_status()

            data = resp.json()

            active = int(data.get("active", 0))
            message = data.get("text", "Không có nội dung")

            return active, message

        except requests.exceptions.Timeout:
            return 0, "❌ Không kết nối được server (timeout)"

        except requests.exceptions.ConnectionError:
            return 0, "❌ Không thể kết nối server"

        except ValueError:
            return 0, "❌ Dữ liệu trả về không hợp lệ (JSON lỗi)"

        except Exception as e:
            return 0, f"❌ Lỗi không xác định:\n{str(e)}"

        
    def xoanhatky(self):
        self.ui.log.clear()

    def load_files_from_folder(self):
        """Tự động load danh sách file trong thư mục 'đơn hàng' khi mở ứng dụng"""
        if not os.path.exists(FOLDER_PATH):  # Nếu thư mục không tồn tại, tạo mới
            os.makedirs(FOLDER_PATH)
            self.ui.log.append(f"Đã tạo thư mục '{FOLDER_PATH}'.")

        # Lọc chỉ lấy file có định dạng hợp lệ
        files = [
            os.path.abspath(os.path.join(FOLDER_PATH, f))
            for f in os.listdir(FOLDER_PATH)
            if os.path.isfile(os.path.join(FOLDER_PATH, f)) and os.path.splitext(f)[1].lower() in ALLOWED_EXTENSIONS
        ]
        
        if files:
            self.file_list = list(set(files))  # Xóa trùng lặp (dự phòng)
            self.model.setStringList(self.file_list)  # Hiển thị cả đường dẫn file
            self.ui.log.append(f"Đã load {len(self.file_list)} file từ '{FOLDER_PATH}'.")
        else:
            # 👉 XÓA DANH SÁCH CŨ
            self.file_list = []
            self.model.setStringList([])
            self.ui.log.append(f"Thư mục '{FOLDER_PATH}' không có file hợp lệ.")

    def dragEnterEvent(self, event):
        """Cho phép nhận file khi kéo vào cửa sổ"""
        if event.mimeData().hasUrls():
            event.acceptProposedAction()

    def dropEvent(self, event):
        """Xử lý khi thả file vào cửa sổ"""
        files = [os.path.abspath(url.toLocalFile()) for url in event.mimeData().urls()]

        # Lọc chỉ lấy file có định dạng hợp lệ
        valid_files = [file for file in files if os.path.splitext(file)[1].lower() in ALLOWED_EXTENSIONS]

        if not valid_files:
            self.ui.log.append("Không có file hợp lệ được thêm.")
            return

        # Kiểm tra và loại bỏ file trùng lặp
        existing_files = set(self.file_list)
        new_files = [file for file in valid_files if file not in existing_files]

        if new_files:
            self.file_list.extend(new_files)
            self.file_list = list(set(self.file_list))  # Xóa trùng lặp lần nữa (dự phòng)
            self.model.setStringList(self.file_list)  # Hiển thị cả đường dẫn file
            self.ui.log.append(f"Đã thêm {len(new_files)} file.")  # Ghi log
        else:
            self.ui.log.append("Không có file mới được thêm (trùng lặp).")

    def xoa_file(self, event):
        """Xóa file trong danh sách khi nhấn phím Delete"""
        if event.key() == Qt.Key_Delete:
            selected_indexes = self.ui.listdanhsach.selectedIndexes()
            if not selected_indexes:
                return  # Không chọn gì thì không làm gì cả

            # Xóa các file được chọn
            files_to_remove = [self.file_list[i.row()] for i in selected_indexes]
            for file in files_to_remove:
                self.file_list.remove(file)

            # Cập nhật lại danh sách
            self.model.setStringList(self.file_list)  # Hiển thị cả đường dẫn file
            self.ui.log.append(f"Đã xóa {len(files_to_remove)} file khỏi danh sách.")

    def xu_ly_don_hang(self):
        """Hàm xử lý khi nhấn nút Xác nhận"""
        if not self.file_list:
            self.ui.log.append("Không có file nào để xử lý!")
            return
        stt = self.ui.stt.text().strip()
  # Đối với QTextEdit, dùng toPlainText()


        if not stt.isdigit():  # Kiểm tra nếu không phải số
            self.ui.log.append("⚠ STT phải là số hợp lệ!")
            return
        self.ui.log.append("🚀 Bắt đầu xử lý...")
        
        try:
            xulydonhang.ProcessHandler.xoa_du_lieu_don_dat_hang()
        except PermissionError:
            self.ui.log.append("⚠️File Excel 'dondathang.xlsx' đang được mở, vui lòng đóng lại để tiếp tục.")
            return
        except Exception as e:
            print(f"❌ Lỗi khác xảy ra: {e}")
            return


        self.ui.tableStatus.setRowCount(0)
        self._lock_ui_for_processing(True)
        open("message.txt", "w").close()


        # Tạo luồng xử lý file

        self.processor = FileProcessorThread(self.file_list, int(stt), self.ui.tableStatus,self.ui.pushMisa)
        self.processor.stt_signal.connect(self.cap_nhat_stt)  # 🚀 Kết nối Signal cập nhật STT
        self.processor.log_signal.connect(self.ui.log.append)  # Ghi log theo thời gian thực
        self.processor.table_signal.connect(self.cap_nhat_table)  # Kết nối tín hiệu bảng
        self.processor.finished_all.connect(self.log_all_done)
        self.processor.start()

    def cap_nhat_stt(self, new_stt):
        """Cập nhật STT trên giao diện"""
        self.ui.stt.setText(str(new_stt))




    def cap_nhat_table(self, filename, page, system,makhachhang, po, saigia, status):
        """Cập nhật dữ liệu vào bảng tableStatus"""
        row_count = self.ui.tableStatus.rowCount()
        self.ui.tableStatus.insertRow(row_count)
        self.ui.tableStatus.setItem(row_count, 0, QTableWidgetItem(filename))
        self.ui.tableStatus.setItem(row_count, 1, QTableWidgetItem(str(page)))
        self.ui.tableStatus.setItem(row_count, 2, QTableWidgetItem(system))
        self.ui.tableStatus.setItem(row_count, 3, QTableWidgetItem(makhachhang))
        self.ui.tableStatus.setItem(row_count, 4, QTableWidgetItem(po))
        self.ui.tableStatus.setItem(row_count, 5, QTableWidgetItem(saigia))
        self.ui.tableStatus.setItem(row_count, 6, QTableWidgetItem(status))
         # Tạo màu sắc nổi bật
        for col in range(6):
            item = self.ui.tableStatus.item(row_count, col)
            
            # Làm đậm chữ
            font = QFont()
            font.setBold(True)
            item.setFont(font)

            # Đổi màu nền dựa trên trạng thái
            if "✅Hoàn Thành" in status:
                item.setBackground(QColor(144, 238, 144))  # Màu xanh lá nhạt (LightGreen)
            elif "❌Thất bại" in status:
                item.setBackground(QColor(255, 102, 102))  # Màu đỏ nhạt
                item.setForeground(QColor(255, 255, 255))  # Chữ màu trắng
            elif "⚠️Hoàn Thành" in status:
                item.setBackground(QColor(255, 223, 102))  # Màu vàng nhạt
            else:
                item.setBackground(QColor(200, 200, 200))  # Màu xám cho trạng thái khác

        header = self.ui.tableStatus.horizontalHeader()
        if header:
            # Ưu tiên các cột còn lại (1 → n) tự điều chỉnh theo nội dung
            for col in range(1, self.ui.tableStatus.columnCount()):
                header.setSectionResizeMode(col, QHeaderView.ResizeToContents)

            # Cột đầu tiên tự co giãn nếu còn trống, nếu không thì giữ nguyên
            header.setSectionResizeMode(0, QHeaderView.Interactive)  # Cho phép kéo chỉnh tay nếu cần
            if self.ui.tableStatus.viewport().width() > header.length():  # Kiểm tra nếu còn không gian trống
                header.setSectionResizeMode(0, QHeaderView.Stretch)  # Cột đầu co giãn nếu còn trống



        # 2) Copy file nếu trạng thái hoàn thành (bỏ qua hệ thống JIT-CHOICE)
        is_done = "✅Hoàn Thành" in status or "⚠️Hoàn Thành" in status
        if is_done and system.strip() != "JIT-CHOICE":
            import shutil, datetime, os

            # Lấy PO và strip whitespace/newline
            po_clean = po.strip()

            # Xác định file gốc
            src = next((f for f in self.file_list if os.path.basename(f) == filename), None)
            if not src or not os.path.isfile(src):
                self.ui.log.append(f"⚠️ Không tìm thấy file gốc: {filename}")
                return

            # Thư mục đích theo tháng-năm
            month_folder = datetime.datetime.now().strftime("%m-%Y")  # e.g. "04-2025"
            dest_dir = os.path.join(FOLDER_PATH, month_folder)
            os.makedirs(dest_dir, exist_ok=True)

            # Đổi tên thành PO + giữ đuôi gốc
            ext = os.path.splitext(src)[1]
            dest = os.path.join(dest_dir, f"{po_clean}{ext}")

            try:
                shutil.copy(src, dest)
                self.ui.log.append(f"✅ Đã copy {filename} → {dest}")
            except Exception as e:
                self.ui.log.append(f"⚠️ Copy thất bại: {e}")


        

    def log_all_done(self):
        """Ghi log khi đã xong tất cả các file"""
        self.ui.log.append("🎉 Hoàn tất tất cả các file!")
        self.export_table_to_log()
        self._lock_ui_for_processing(False)

if __name__ == "__main__":


    

    if is_already_running():
        print("Chương trình đã chạy, đưa lên top...")
    if bring_window_to_top(window_title):
        sys.exit()  # Thoát ngay nếu chương trình đã chạy
    else:
        print("Không tìm thấy cửa sổ chương trình.")
        
    app = QApplication(sys.argv)
    window = MyApp()
    window.show()
    sys.exit(app.exec())