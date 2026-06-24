# -*- coding: utf-8 -*-

from PySide6.QtCore import (QCoreApplication, QDate, QDateTime, QLocale,
    QMetaObject, QObject, QPoint, QRect,
    QSize, QTime, QUrl, Qt)
from PySide6.QtGui import (QBrush, QColor, QConicalGradient, QCursor,
    QFont, QFontDatabase, QGradient, QIcon,
    QImage, QKeySequence, QLinearGradient, QPainter,
    QPalette, QPixmap, QRadialGradient, QTransform)
from PySide6.QtWidgets import (QAbstractItemView, QApplication, QGroupBox, QHeaderView,
    QLabel, QLineEdit, QListView, QMainWindow, QHBoxLayout, QVBoxLayout,
    QPushButton, QSizePolicy, QStatusBar, QTabWidget,
    QTableWidget, QTableWidgetItem, QTextEdit, QWidget, QFrame, QSplitter)

class Ui_MainWindow(object):
    def setupUi(self, MainWindow):
        if not MainWindow.objectName():
            MainWindow.setObjectName(u"MainWindow")
        MainWindow.resize(950, 850)
        MainWindow.setMinimumSize(QSize(900, 800))
        
        # --- Bảng màu & Phong cách Modern UI ---
        style_sheet = """
            QMainWindow {
                background-color: #f5f7fa;
            }
            QTabWidget::pane {
                border: 1px solid #dcdfe6;
                background: white;
                border-radius: 8px;
                top: -1px;
            }
            QTabBar::tab {
                background: #e4e7ed;
                border: 1px solid #dcdfe6;
                padding: 10px 20px;
                border-top-left-radius: 8px;
                border-top-right-radius: 8px;
                margin-right: 2px;
                font-weight: bold;
                color: #606266;
            }
            QTabBar::tab:selected {
                background: white;
                border-bottom-color: white;
                color: #409eff;
            }
            QGroupBox {
                font-weight: bold;
                border: 1px solid #dcdfe6;
                border-radius: 10px;
                margin-top: 15px;
                padding-top: 10px;
                background-color: white;
            }
            QGroupBox::title {
                subcontrol-origin: margin;
                subcontrol-position: top left;
                left: 15px;
                padding: 0 5px;
                color: #303133;
            }
            QPushButton {
                border-radius: 6px;
                padding: 8px 15px;
                font-weight: bold;
                font-size: 13px;
                min-height: 30px;
            }
            #xacnhan {
                background-color: #67c23a;
                color: white;
                border: none;
            }
            #xacnhan:hover { background-color: #85ce61; }
            #xacnhan:pressed { background-color: #529b2e; }

            #loadfile {
                background-color: #409eff;
                color: white;
                border: none;
            }
            #loadfile:hover { background-color: #66b1ff; }

            #zalo_btn {
                background-color: #0068ff;
                color: white;
                border: none;
            }

            QLineEdit {
                border: 1px solid #dcdfe6;
                border-radius: 4px;
                padding: 5px;
                background: white;
            }
            QLineEdit:focus {
                border-color: #409eff;
            }

            QTableWidget {
                border: 1px solid #dcdfe6;
                gridline-color: #ebeef5;
                border-radius: 4px;
            }
            QHeaderView::section {
                background-color: #f5f7fa;
                padding: 4px;
                border: 1px solid #dcdfe6;
                font-weight: bold;
            }
            QTextEdit {
                border: 1px solid #dcdfe6;
                border-radius: 4px;
                background-color: #fafafa;
                font-family: 'Consolas';
                font-size: 10pt;
            }
        """
        MainWindow.setStyleSheet(style_sheet)
        
        self.centralwidget = QWidget(MainWindow)
        self.main_layout = QVBoxLayout(self.centralwidget)
        self.main_layout.setContentsMargins(15, 15, 15, 15)
        self.main_layout.setSpacing(10)

        # --- Tab Widget Chính ---
        self.tabWidget = QTabWidget(self.centralwidget)
        
        # --- TAB 1: XỬ LÝ ĐƠN HÀNG ---
        self.tab_process = QWidget()
        self.layout_tab1 = QVBoxLayout(self.tab_process)

        # 1. Khu vực điều khiển (STT & Nút bấm)
        self.top_control_frame = QFrame()
        self.layout_top = QHBoxLayout(self.top_control_frame)
        self.layout_top.setContentsMargins(0, 0, 0, 0)

        self.group_files = QGroupBox("1. Danh sách file đầu vào")
        self.layout_files = QVBoxLayout(self.group_files)
        self.listdanhsach = QListView()
        self.layout_files.addWidget(self.listdanhsach)
        
        self.loadfile = QPushButton("🔄 Tải lại đơn hàng")
        self.loadfile.setObjectName("loadfile")
        self.layout_files.addWidget(self.loadfile)

        self.group_config = QGroupBox("2. Cấu hình & Thực thi")
        self.layout_config = QVBoxLayout(self.group_config)
        
        self.label_stt = QLabel("Số thứ tự đơn hàng bắt đầu:")
        self.stt = QLineEdit()
        self.stt.setPlaceholderText("Nhập STT...")
        self.stt.setAlignment(Qt.AlignCenter)
        self.stt.setMinimumHeight(40)
        font_stt = QFont(); font_stt.setPointSize(12); font_stt.setBold(True)
        self.stt.setFont(font_stt)

        self.xacnhan = QPushButton("🚀 XỬ LÝ ĐƠN HÀNG")
        self.xacnhan.setObjectName("xacnhan")
        self.xacnhan.setMinimumHeight(60)

        self.zalo_btn = QPushButton("📱 Gửi thông báo Zalo")
        self.zalo_btn.setObjectName("zalo_btn")

        self.layout_config.addWidget(self.label_stt)
        self.layout_config.addWidget(self.stt)
        self.layout_config.addStretch()
        self.layout_config.addWidget(self.zalo_btn)
        self.layout_config.addWidget(self.xacnhan)

        self.layout_top.addWidget(self.group_files, 3)
        self.layout_top.addWidget(self.group_config, 1)
        self.layout_tab1.addWidget(self.top_control_frame, 2)

        # 2. Nhật ký & Bảng trạng thái (Splitter co giãn)
        self.splitter = QSplitter(Qt.Vertical)
        
        self.group_log = QGroupBox("3. Nhật ký hệ thống")
        self.layout_log = QVBoxLayout(self.group_log)
        self.log = QTextEdit()
        self.log.setReadOnly(True)
        self.layout_log.addWidget(self.log)

        self.group_status = QGroupBox("4. Kết quả xử lý chi tiết")
        self.layout_status = QVBoxLayout(self.group_status)
        self.tableStatus = QTableWidget()
        self.tableStatus.setAlternatingRowColors(True)
        self.layout_status.addWidget(self.tableStatus)

        self.splitter.addWidget(self.group_log)
        self.splitter.addWidget(self.group_status)
        self.layout_tab1.addWidget(self.splitter, 5)

        self.tabWidget.addTab(self.tab_process, "Xử lý Đơn hàng")

        # --- TAB 2: THÔNG TIN ---
        self.tab_info = QWidget()
        self.layout_info = QVBoxLayout(self.tab_info)
        
        self.info_box = QGroupBox("Về phần mềm")
        self.layout_info_content = QVBoxLayout(self.info_box)
        
        self.info_text = QLabel(
            "<h2>AUTOMATED ORDER PROCESSING SYSTEM</h2>"
            "<p><b>Chức năng chính:</b></p>"
            "<ul>"
            "<li>Phân tích đơn hàng PDF/XLSX/TXT từ hệ thống MT (BigC, Lotte, Satra...)</li>"
            "<li>Tự động đối soát giá bán và chương trình khuyến mãi.</li>"
            "<li>Xuất dữ liệu chuẩn hóa phục vụ kế toán.</li>"
            "</ul>"
            "<p><b>Tác giả:</b> HUYNH DAT THANH</p>"
            "<p><b>Liên hệ:</b> 0947.940.391 | byun.huynh@gmail.com</p>"
        )
        self.info_text.setWordWrap(True)
        self.info_text.setAlignment(Qt.AlignTop)
        
        self.qr_label = QLabel()
        self.qr_label.setPixmap(QPixmap(u"qr.jpg").scaled(200, 200, Qt.KeepAspectRatio, Qt.SmoothTransformation))
        self.qr_label.setAlignment(Qt.AlignCenter)

        self.layout_info_content.addWidget(self.info_text)
        self.layout_info_content.addWidget(self.qr_label)
        self.layout_info.addWidget(self.info_box)
        self.layout_info.addStretch()

        self.tabWidget.addTab(self.tab_info, "Thông tin")

        # --- Các thành phần ẩn (Dành cho logic cũ tương thích) ---
        self.groupBox_6 = QGroupBox(); self.groupBox_6.hide()
        self.traHang = QTableWidget()
        self.pushMisa = QPushButton("Push Misa")

        # --- Footer ---
        self.label_footer = QLabel("© 2025 Blue Hà Thành. All rights reserved.")
        self.label_footer.setStyleSheet("color: #909399; font-size: 11px;")
        self.label_footer.setAlignment(Qt.AlignCenter)

        self.main_layout.addWidget(self.tabWidget)
        self.main_layout.addWidget(self.label_footer)

        MainWindow.setCentralWidget(self.centralwidget)
        self.statusbar = QStatusBar(MainWindow)
        MainWindow.setStatusBar(self.statusbar)

        self.retranslateUi(MainWindow)
        QMetaObject.connectSlotsByName(MainWindow)

    def retranslateUi(self, MainWindow):
        MainWindow.setWindowTitle("Blue Hà Thành - Order System v2.0")