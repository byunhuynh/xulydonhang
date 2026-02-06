# -*- coding: utf-8 -*-

################################################################################
## Form generated from reading UI file 'form.ui'
##
## Created by: Qt User Interface Compiler version 6.8.2
##
## WARNING! All changes made in this file will be lost when recompiling UI file!
################################################################################

from PySide6.QtCore import (QCoreApplication, QDate, QDateTime, QLocale,
    QMetaObject, QObject, QPoint, QRect,
    QSize, QTime, QUrl, Qt)
from PySide6.QtGui import (QBrush, QColor, QConicalGradient, QCursor,
    QFont, QFontDatabase, QGradient, QIcon,
    QImage, QKeySequence, QLinearGradient, QPainter,
    QPalette, QPixmap, QRadialGradient, QTransform)
from PySide6.QtWidgets import (QAbstractItemView, QApplication, QGroupBox, QHeaderView,
    QLabel, QLineEdit, QListView, QMainWindow,
    QPushButton, QSizePolicy, QStatusBar, QTabWidget,
    QTableWidget, QTableWidgetItem, QTextEdit, QWidget)

class Ui_MainWindow(object):
    def setupUi(self, MainWindow):
        if not MainWindow.objectName():
            MainWindow.setObjectName(u"MainWindow")
        MainWindow.resize(780, 750)
        MainWindow.setMinimumSize(QSize(780, 750))
        MainWindow.setMaximumSize(QSize(780, 750))
        icon = QIcon()
        icon.addFile(u"blue.ico", QSize(), QIcon.Mode.Normal, QIcon.State.Off)
        MainWindow.setWindowIcon(icon)
        MainWindow.setLayoutDirection(Qt.LayoutDirection.LeftToRight)
        MainWindow.setStyleSheet(u"")
        MainWindow.setLocale(QLocale(QLocale.Vietnamese, QLocale.Vietnam))
        MainWindow.setUnifiedTitleAndToolBarOnMac(False)
        self.centralwidget = QWidget(MainWindow)
        self.centralwidget.setObjectName(u"centralwidget")
        self.groupBox_6 = QGroupBox(self.centralwidget)
        self.groupBox_6.setObjectName(u"groupBox_6")
        self.groupBox_6.setGeometry(QRect(740, 660, 721, 101))
        self.traHang = QTableWidget(self.groupBox_6)
        self.traHang.setObjectName(u"traHang")
        self.traHang.setGeometry(QRect(640, 20, 71, 31))
        self.pushMisa = QPushButton(self.groupBox_6)
        self.pushMisa.setObjectName(u"pushMisa")
        self.pushMisa.setGeometry(QRect(620, 20, 91, 41))
        self.pushMisa.setAutoDefault(True)
        self.pushMisa.setFlat(False)
        self.tabWidget = QTabWidget(self.centralwidget)
        self.tabWidget.setObjectName(u"tabWidget")
        self.tabWidget.setGeometry(QRect(0, 0, 771, 701))
        self.tab = QWidget()
        self.tab.setObjectName(u"tab")
        self.groupBox_7 = QGroupBox(self.tab)
        self.groupBox_7.setObjectName(u"groupBox_7")
        self.groupBox_7.setGeometry(QRect(10, 0, 741, 671))
        self.groupBox_7.setStyleSheet(u"QGroupBox {\n"
"        border: 2px solid rgb(134, 134, 134);\n"
"        border-radius: 10px;\n"
"        margin-top: 0px;\n"
"        padding: 0px;\n"
"    }\n"
"    QGroupBox::title {\n"
"        subcontrol-origin: margin;\n"
"        left: 0px;\n"
"        padding: 0px;\n"
"    }")
        self.groupBox_7.setLocale(QLocale(QLocale.Vietnamese, QLocale.Vietnam))
        self.groupBox_7.setFlat(False)
        self.groupBox_7.setCheckable(False)
        self.groupBox_9 = QGroupBox(self.groupBox_7)
        self.groupBox_9.setObjectName(u"groupBox_9")
        self.groupBox_9.setGeometry(QRect(10, 220, 721, 211))
        self.log = QTextEdit(self.groupBox_9)
        self.log.setObjectName(u"log")
        self.log.setEnabled(True)
        self.log.setGeometry(QRect(10, 20, 701, 181))
        self.log.setStyleSheet(u" QTextEdit {\n"
"        border: 2px solid #607D8B;\n"
"        border-radius: 10px;\n"
"        background-color: #ECEFF1;\n"
"        padding: 8px;\n"
"        font-family: Consolas, monospace;\n"
"        font-size: 13px;\n"
"        color: #263238;\n"
"    }")
        self.log.setUndoRedoEnabled(False)
        self.log.setReadOnly(True)
        self.groupBox_5 = QGroupBox(self.groupBox_7)
        self.groupBox_5.setObjectName(u"groupBox_5")
        self.groupBox_5.setGeometry(QRect(10, 440, 721, 221))
        self.tableStatus = QTableWidget(self.groupBox_5)
        self.tableStatus.setObjectName(u"tableStatus")
        self.tableStatus.setGeometry(QRect(10, 20, 701, 191))
        self.tableStatus.setStyleSheet(u"\n"
"    QTableWidget {\n"
"        border: 2px solid #2196F3;\n"
"        border-radius: 10px;\n"
"        background-color: #f0f0f0;\n"
"        padding: 5px;\n"
"    }\n"
"    \n"
"    QTableWidget::item {\n"
"        padding: 5px;\n"
"        border-bottom: 1px solid #ccc;\n"
"    }\n"
"\n"
"    QTableWidget::item:selected {\n"
"        background-color: #cce5ff;\n"
"        color: black;\n"
"    }\n"
"\n"
"")
        self.tableStatus.setEditTriggers(QAbstractItemView.EditTrigger.NoEditTriggers)
        self.tabWidget_2 = QTabWidget(self.groupBox_7)
        self.tabWidget_2.setObjectName(u"tabWidget_2")
        self.tabWidget_2.setGeometry(QRect(10, 10, 721, 211))
        self.tab_3 = QWidget()
        self.tab_3.setObjectName(u"tab_3")
        self.groupBox_8 = QGroupBox(self.tab_3)
        self.groupBox_8.setObjectName(u"groupBox_8")
        self.groupBox_8.setGeometry(QRect(0, 0, 711, 181))
        self.groupBox = QGroupBox(self.groupBox_8)
        self.groupBox.setObjectName(u"groupBox")
        self.groupBox.setGeometry(QRect(10, 20, 531, 151))
        self.listdanhsach = QListView(self.groupBox)
        self.listdanhsach.setObjectName(u"listdanhsach")
        self.listdanhsach.setGeometry(QRect(10, 20, 511, 91))
        self.listdanhsach.setStyleSheet(u"\n"
"    QListView {\n"
"        border: 2px solid #2196F3;\n"
"        border-radius: 10px;\n"
"        background-color: #f0f0f0;\n"
"        padding: 5px;\n"
"    }\n"
"    \n"
"    QListView::item {\n"
"        padding: 5px;\n"
"        border-bottom: 1px solid #ccc;\n"
"    }\n"
"\n"
"    QListView::item:selected {\n"
"        background-color: #cce5ff;\n"
"        color: black;\n"
"    }\n"
"\n"
"")
        self.loadfile = QPushButton(self.groupBox)
        self.loadfile.setObjectName(u"loadfile")
        self.loadfile.setGeometry(QRect(190, 115, 161, 31))
        self.loadfile.setStyleSheet(u"  QPushButton {\n"
"        background-color: #FF9800;\n"
"        color: white;\n"
"        border: none;\n"
"        border-radius: 20px;\n"
"        padding: 0 20px;\n"
"        font-size: 14px;\n"
"        font-weight: bold;\n"
"    }\n"
"\n"
"    QPushButton:hover {\n"
"        background-color: #FB8C00;\n"
"    }\n"
"\n"
"    QPushButton:pressed {\n"
"        background-color: #EF6C00;\n"
"    }")
        icon1 = QIcon()
        icon1.addFile(u"icons8_synchronize_2.ico", QSize(), QIcon.Mode.Normal, QIcon.State.Off)
        self.loadfile.setIcon(icon1)
        self.groupBox_3 = QGroupBox(self.groupBox_8)
        self.groupBox_3.setObjectName(u"groupBox_3")
        self.groupBox_3.setGeometry(QRect(550, 10, 151, 81))
        self.stt = QLineEdit(self.groupBox_3)
        self.stt.setObjectName(u"stt")
        self.stt.setGeometry(QRect(10, 20, 131, 51))
        self.stt.setStyleSheet(u"QLineEdit {\n"
"        border: 2px solid #9C27B0;\n"
"        border-radius: 12px;\n"
"        padding: 8px;\n"
"        background-color: #fff;\n"
"        color: #333;\n"
"        font-size: 14px;\n"
"    }")
        self.xacnhan = QPushButton(self.groupBox_8)
        self.xacnhan.setObjectName(u"xacnhan")
        self.xacnhan.setGeometry(QRect(550, 100, 151, 71))
        font = QFont()
        font.setFamilies([u"Arial"])
        font.setBold(True)
        font.setItalic(True)
        self.xacnhan.setFont(font)
        self.xacnhan.setStyleSheet(u" QPushButton {\n"
"        background-color: #4CAF50;\n"
"        color: white;\n"
"        border: none;\n"
"        border-radius: 20px;\n"
"        padding: 0 20px;\n"
"        font-size: 14px;\n"
"        font-weight: bold;\n"
"        letter-spacing: 1px;\n"
"    }\n"
"\n"
"    QPushButton:hover {\n"
"        background-color: #43A047;\n"
"    }\n"
"\n"
"    QPushButton:pressed {\n"
"        background-color: #388E3C;\n"
"    }")
        icon2 = QIcon()
        icon2.addFile(u"icons8_edit.ico", QSize(), QIcon.Mode.Normal, QIcon.State.Off)
        self.xacnhan.setIcon(icon2)
        self.xacnhan.setIconSize(QSize(30, 30))
        self.tabWidget_2.addTab(self.tab_3, "")
        self.tab_4 = QWidget()
        self.tab_4.setObjectName(u"tab_4")
        self.groupBox_15 = QGroupBox(self.tab_4)
        self.groupBox_15.setObjectName(u"groupBox_15")
        self.groupBox_15.setGeometry(QRect(0, 0, 711, 181))
        self.zalo_btn = QPushButton(self.groupBox_15)
        self.zalo_btn.setObjectName(u"zalo_btn")
        self.zalo_btn.setGeometry(QRect(200, 60, 231, 71))
        self.zalo_btn.setFont(font)
        self.zalo_btn.setStyleSheet(u" QPushButton {\n"
"        background-color: #4CAF50;\n"
"        color: white;\n"
"        border: none;\n"
"        border-radius: 22px;\n"
"        padding: 0 25px;\n"
"        font-size: 16px;\n"
"        font-weight: bold;\n"
"        letter-spacing: 1px;\n"
"    }\n"
"\n"
"    QPushButton:hover {\n"
"        background-color: #43A047;\n"
"    }\n"
"\n"
"    QPushButton:pressed {\n"
"        background-color: #388E3C;\n"
"    }")
        icon3 = QIcon(QIcon.fromTheme(QIcon.ThemeIcon.MailSend))
        self.zalo_btn.setIcon(icon3)
        self.zalo_btn.setIconSize(QSize(30, 30))
        self.tabWidget_2.addTab(self.tab_4, "")
        self.tabWidget.addTab(self.tab, "")
        self.tab_2 = QWidget()
        self.tab_2.setObjectName(u"tab_2")
        self.groupBox_10 = QGroupBox(self.tab_2)
        self.groupBox_10.setObjectName(u"groupBox_10")
        self.groupBox_10.setGeometry(QRect(10, 0, 741, 671))
        self.groupBox_10.setStyleSheet(u"QGroupBox {\n"
"        border: 2px solid rgb(134, 134, 134);\n"
"        border-radius: 10px;\n"
"        margin-top: 0px;\n"
"        padding: 0px;\n"
"    }\n"
"    QGroupBox::title {\n"
"        subcontrol-origin: margin;\n"
"        left: 0px;\n"
"        padding: 0px;\n"
"    }")
        self.groupBox_10.setLocale(QLocale(QLocale.Vietnamese, QLocale.Vietnam))
        self.groupBox_10.setFlat(False)
        self.groupBox_10.setCheckable(False)
        self.groupBox_11 = QGroupBox(self.groupBox_10)
        self.groupBox_11.setObjectName(u"groupBox_11")
        self.groupBox_11.setGeometry(QRect(10, 10, 721, 351))
        self.groupBox_2 = QGroupBox(self.groupBox_11)
        self.groupBox_2.setObjectName(u"groupBox_2")
        self.groupBox_2.setGeometry(QRect(10, 20, 701, 121))
        self.groupBox_2.setStyleSheet(u"")
        self.label = QLabel(self.groupBox_2)
        self.label.setObjectName(u"label")
        self.label.setGeometry(QRect(10, 20, 471, 101))
        self.label.setStyleSheet(u"")
        self.groupBox_4 = QGroupBox(self.groupBox_11)
        self.groupBox_4.setObjectName(u"groupBox_4")
        self.groupBox_4.setGeometry(QRect(10, 150, 701, 191))
        self.label_2 = QLabel(self.groupBox_4)
        self.label_2.setObjectName(u"label_2")
        self.label_2.setGeometry(QRect(10, 20, 641, 161))
        self.groupBox_13 = QGroupBox(self.groupBox_10)
        self.groupBox_13.setObjectName(u"groupBox_13")
        self.groupBox_13.setGeometry(QRect(10, 370, 721, 291))
        self.groupBox_12 = QGroupBox(self.groupBox_13)
        self.groupBox_12.setObjectName(u"groupBox_12")
        self.groupBox_12.setGeometry(QRect(10, 20, 271, 261))
        self.label_3 = QLabel(self.groupBox_12)
        self.label_3.setObjectName(u"label_3")
        self.label_3.setGeometry(QRect(10, 30, 201, 171))
        self.groupBox_14 = QGroupBox(self.groupBox_13)
        self.groupBox_14.setObjectName(u"groupBox_14")
        self.groupBox_14.setGeometry(QRect(290, 20, 421, 261))
        self.label_5 = QLabel(self.groupBox_14)
        self.label_5.setObjectName(u"label_5")
        self.label_5.setGeometry(QRect(20, 20, 381, 231))
        self.label_5.setPixmap(QPixmap(u"qr.jpg"))
        self.label_5.setScaledContents(True)
        self.tabWidget.addTab(self.tab_2, "")
        self.label_6 = QLabel(self.centralwidget)
        self.label_6.setObjectName(u"label_6")
        self.label_6.setGeometry(QRect(240, 710, 261, 16))
        MainWindow.setCentralWidget(self.centralwidget)
        self.statusbar = QStatusBar(MainWindow)
        self.statusbar.setObjectName(u"statusbar")
        MainWindow.setStatusBar(self.statusbar)

        self.retranslateUi(MainWindow)

        self.pushMisa.setDefault(False)
        self.tabWidget.setCurrentIndex(0)
        self.tabWidget_2.setCurrentIndex(0)


        QMetaObject.connectSlotsByName(MainWindow)
    # setupUi

    def retranslateUi(self, MainWindow):
        MainWindow.setWindowTitle(QCoreApplication.translate("MainWindow", u"X\u1eed l\u00fd \u0111\u01a1n h\u00e0ng - Blue H\u00e0 Th\u00e0nh", None))
        self.groupBox_6.setTitle(QCoreApplication.translate("MainWindow", u"Tr\u1ea3 h\u00e0ng", None))
        self.pushMisa.setText(QCoreApplication.translate("MainWindow", u"\u0110\u1ea9y \u0111\u01a1n Misa", None))
        self.groupBox_7.setTitle("")
        self.groupBox_9.setTitle(QCoreApplication.translate("MainWindow", u"Nh\u1eadt k\u00fd X\u1eed l\u00fd", None))
        self.groupBox_5.setTitle(QCoreApplication.translate("MainWindow", u"T\u00ecnh tr\u1ea1ng \u0110\u01a1n h\u00e0ng", None))
        self.groupBox_8.setTitle(QCoreApplication.translate("MainWindow", u"Th\u00f4ng tin v\u00e0 X\u1eed l\u00fd \u0111\u01a1n h\u00e0ng", None))
        self.groupBox.setTitle(QCoreApplication.translate("MainWindow", u"Danh s\u00e1ch File", None))
        self.loadfile.setText(QCoreApplication.translate("MainWindow", u"T\u1ea3i l\u1ea1i \u0111\u01a1n h\u00e0ng", None))
        self.groupBox_3.setTitle(QCoreApplication.translate("MainWindow", u"S\u1ed1 th\u1ee9 t\u1ef1 \u0111\u01a1n h\u00e0ng", None))
        self.xacnhan.setText(QCoreApplication.translate("MainWindow", u"X\u1eed l\u00fd\n"
"\u0110\u01a1n h\u00e0ng", None))
        self.tabWidget_2.setTabText(self.tabWidget_2.indexOf(self.tab_3), QCoreApplication.translate("MainWindow", u"X\u1eed l\u00fd", None))
        self.groupBox_15.setTitle(QCoreApplication.translate("MainWindow", u"G\u1eedi Th\u00f4ng b\u00e1o Zalo", None))
        self.zalo_btn.setText(QCoreApplication.translate("MainWindow", u"G\u1eedi Th\u00f4ng B\u00e1o\n"
"ZALO", None))
        self.tabWidget_2.setTabText(self.tabWidget_2.indexOf(self.tab_4), QCoreApplication.translate("MainWindow", u"G\u1eedi Zalo", None))
        self.tabWidget.setTabText(self.tabWidget.indexOf(self.tab), QCoreApplication.translate("MainWindow", u"X\u1eed l\u00fd \u0110\u01a1n h\u00e0ng", None))
        self.groupBox_10.setTitle("")
        self.groupBox_11.setTitle(QCoreApplication.translate("MainWindow", u"Th\u00f4ng tin", None))
        self.groupBox_2.setTitle(QCoreApplication.translate("MainWindow", u"PH\u1ea6N M\u1ec0M X\u1eec L\u00dd \u0110\u01a0N H\u00c0NG \u2013 AUTOMATED ORDER PROCESSING SYSTEM", None))
        self.label.setText(QCoreApplication.translate("MainWindow", u"<html><head/><body><p>Ph\u1ea7n m\u1ec1m h\u1ed7 tr\u1ee3 x\u1eed l\u00fd to\u00e0n b\u1ed9 \u0111\u01a1n h\u00e0ng t\u1eeb nhi\u1ec1u h\u1ec7 th\u1ed1ng:</p><p>\u2022 MT (Modern Trade): Big C / GO!, Lotte Mart, Satra, Kingfood, Winmart, Fujimart, \u2026</p><p>\u2022 TM\u0110T (E-commerce): Shopee, TikTok Shop, \u2026</p></body></html>", None))
        self.groupBox_4.setTitle(QCoreApplication.translate("MainWindow", u"Ch\u1ee9c n\u0103ng bao g\u1ed3m: ", None))
        self.label_2.setText(QCoreApplication.translate("MainWindow", u"<html><head/><body><p>\u2022 T\u1ef1 \u0111\u1ed9ng \u0111\u1ecdc &amp; ph\u00e2n t\u00edch \u0111\u01a1n h\u00e0ng theo nhi\u1ec1u \u0111\u1ecbnh d\u1ea1ng: PDF, TXT, XLSX</p><p>\u2022 T\u1ef1 check gi\u00e1 b\u00e1n, ch\u01b0\u01a1ng tr\u00ecnh khuy\u1ebfn m\u00e3i, sai gi\u00e1, sai s\u1ea3n ph\u1ea9m</p><p>\u2022 T\u1ed5ng h\u1ee3p d\u1eef li\u1ec7u \u2013 xu\u1ea5t b\u00e1o c\u00e1o</p><p>\u2022 G\u1eedi th\u00f4ng b\u00e1o \u0111\u01a1n \u0111\u00e3 x\u1eed l\u00fd, c\u1ea3nh b\u00e1o sai gi\u00e1 qua Zalo</p><p>\u2022 T\u1ef1 \u0111\u1ed9ng h\u00f3a lu\u1ed3ng x\u1eed l\u00fd h\u00e0ng ng\u00e0y nh\u1eb1m gi\u1ea3m thao t\u00e1c th\u1ee7 c\u00f4ng v\u00e0 t\u0103ng \u0111\u1ed9 ch\u00ednh x\u00e1c</p><p>Ph\u1ea7n m\u1ec1m \u0111\u01b0\u1ee3c ph\u00e1t tri\u1ec3n v\u00e0 t\u1ed1i \u01b0u ri\u00eang cho nhu c\u1ea7u v\u1eadn h\u00e0nh th\u1ef1c t\u1ebf.</p></body></html>", None))
        self.groupBox_13.setTitle(QCoreApplication.translate("MainWindow", u"T\u00ecnh tr\u1ea1ng \u0110\u01a1n h\u00e0ng", None))
        self.groupBox_12.setTitle(QCoreApplication.translate("MainWindow", u"Th\u00f4ng tin t\u00e1c gi\u1ea3:", None))
        self.label_3.setText(QCoreApplication.translate("MainWindow", u"<html><head/><body><p>T\u00e1c gi\u1ea3 &amp; s\u1edf h\u1eefu b\u1ea3n quy\u1ec1n:</p><p>HU\u1ef2NH \u0110\u1ea0T TH\u00c0NH</p><p><br/></p><p>\u0110i\u1ec7n tho\u1ea1i: 0947.940.391</p><p>Email: byun.huynh@gmail.com</p></body></html>", None))
        self.groupBox_14.setTitle(QCoreApplication.translate("MainWindow", u"Zalo", None))
        self.label_5.setText("")
        self.tabWidget.setTabText(self.tabWidget.indexOf(self.tab_2), QCoreApplication.translate("MainWindow", u"Th\u00f4ng tin", None))
        self.label_6.setText(QCoreApplication.translate("MainWindow", u"\u00a92025 HU\u1ef2NH \u0110\u1ea0T TH\u00c0NH. All rights reserved.\n"
"", None))
    # retranslateUi

