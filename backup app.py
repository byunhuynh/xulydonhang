import sys
import os
from PySide6.QtWidgets import QApplication, QMainWindow, QListView, QMessageBox
from PySide6.QtCore import Qt, QStringListModel
from form_ui import Ui_MainWindow  # Import giao diện từ file UI
from PySide6.QtCore import QThread, Signal
import xulydonhang
from PySide6.QtWidgets import QTableWidget, QTableWidgetItem
import pygetwindow as gw
import pyautogui
import time
import psutil
FOLDER_PATH = os.path.abspath("đơn hàng")  # Chuyển "đơn hàng" sang đường dẫn tuyệt đối
ALLOWED_EXTENSIONS = {".pdf", ".xlsx", ".txt"}  # Chỉ nhận các file có định dạng này


window_title = "Xử lý đơn hàng - Blue Hà Thành"
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

    def __init__(self, file_list,stt):
        super().__init__()
        self.file_list = file_list
        self.stt = stt  # Lưu STT để sử dụng
        xulydonhang.process_handler.log_signal.connect(self.send_log)  # Nhận log từ xulydonhang
        
         # Đặt chính sách kích thước tự động mở rộng
        

   
    def send_log(self, message):
        """Nhận log từ xulydonhang và gửi về giao diện"""
        self.log_signal.emit(message)
        


    def run(self):
        """Chạy lần lượt từng file"""
        for file in self.file_list:
            xulydonhang.process_handler.process_file(file, self.stt)  # Truyền STT vào xử lý
            self.stt = self.stt + 1
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
        self.ui.tableStatus.setColumnCount(5)  # 2 cột: Tên file & Trạng thái
        self.ui.tableStatus.setHorizontalHeaderLabels(["Tên file","Trang","Hệ thống","PO", "Trạng thái"])
        

        # Kích hoạt kéo thả file
        self.ui.listdanhsach.setAcceptDrops(True)
        self.setAcceptDrops(True)

        # Load file khi mở ứng dụng
        self.load_files_from_folder()
        stt = xulydonhang.lay_gia_tri_G1() +1 
        self.ui.stt.setText(str(stt))  # Chuyển giá trị sang chuỗi trước khi đặt vào QLabel

        # Xử lý sự kiện
        self.ui.xacnhan.clicked.connect(self.xu_ly_don_hang)  # Xử lý khi nhấn nút
        self.ui.listdanhsach.keyPressEvent = self.xoa_file  # Nhấn Delete để xóa file khỏi danh sách

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
        stt = self.ui.stt.toPlainText().strip()  # Đối với QTextEdit, dùng toPlainText()


        if not stt.isdigit():  # Kiểm tra nếu không phải số
            self.ui.log.append("⚠ STT phải là số hợp lệ!")
            return
        self.ui.log.append("🚀 Bắt đầu xử lý...")
        xulydonhang.ProcessHandler.xoa_du_lieu_don_dat_hang()
        self.ui.xacnhan.setEnabled(False)  # Khóa button
        self.ui.listdanhsach.setEnabled(False)
        self.ui.stt.setEnabled(False)

        # Tạo luồng xử lý file

        self.processor = FileProcessorThread(self.file_list, int(stt))  # Truyền STT vào
        self.processor.log_signal.connect(self.ui.log.append)  # Ghi log theo thời gian thực
        self.processor.finished_all.connect(self.log_all_done)
        self.processor.start()

    def log_all_done(self):
        """Ghi log khi đã xong tất cả các file"""
        self.ui.log.append("🎉 Hoàn tất tất cả các file!")
        self.ui.xacnhan.setEnabled(True)  # Mở khóa button
        self.ui.listdanhsach.setEnabled(True)
        self.ui.stt.setEnabled(True)

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