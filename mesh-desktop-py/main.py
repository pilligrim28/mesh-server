"""
Meshtastic Monitor Desktop - PySide6 приложение
"""
import sys
import os
from datetime import datetime
from typing import Optional

from PySide6.QtWidgets import (
    QApplication, QMainWindow, QWidget, QVBoxLayout, QHBoxLayout,
    QTabWidget, QLabel, QPushButton, QListWidget, QListWidgetItem,
    QTextEdit, QLineEdit, QFormLayout, QGroupBox, QSplitter,
    QStatusBar, QToolBar, QMessageBox, QFrame, QScrollArea,
    QSpacerItem, QSizePolicy, QProgressBar
)
from PySide6.QtCore import Qt, QTimer, QUrl, Signal, QObject
from PySide6.QtGui import QAction, QIcon, QFont, QBrush, QColor

from api_client import MeshServerClient, Device, Metric, Alert, Message


class WorkerSignals(QObject):
    """Сигналы для воркеров"""
    devices_ready = Signal(list)
    metrics_ready = Signal(list)
    alerts_ready = Signal(list)
    messages_ready = Signal(list)
    status_ready = Signal(dict)
    error = Signal(str)


class MainWindow(QMainWindow):
    def __init__(self):
        super().__init__()
        self.client = MeshServerClient()
        self.signals = WorkerSignals()
        self.refresh_timer = QTimer()
        self.refresh_timer.timeout.connect(self.refresh_data)
        
        self.devices: list[Device] = []
        self.metrics: list[Metric] = []
        self.alerts: list[Alert] = []
        self.messages: list[Message] = []
        
        self.init_ui()
        self.check_connection()
        
    def init_ui(self):
        """Инициализация UI"""
        self.setWindowTitle("Meshtastic Monitor")
        self.setMinimumSize(1200, 800)
        
        # Central widget
        central = QWidget()
        self.setCentralWidget(central)
        main_layout = QHBoxLayout(central)
        main_layout.setContentsMargins(0, 0, 0, 0)
        
        # Map placeholder (слева)
        self.map_widget = QWidget()
        self.map_widget.setMinimumWidth(600)
        self.map_widget.setStyleSheet("background-color: #1a1a2e;")
        map_layout = QVBoxLayout(self.map_widget)
        self.map_label = QLabel("🗺️ Карта расположений")
        self.map_label.setAlignment(Qt.AlignCenter)
        self.map_label.setStyleSheet("""
            QLabel {
                color: #4ecca3;
                font-size: 24px;
                background: #1a1a2e;
            }
        """)
        map_layout.addWidget(self.map_label)
        
        # Sidebar (справа)
        sidebar = QWidget()
        sidebar.setMaximumWidth(450)
        sidebar.setMinimumWidth(350)
        sidebar.setStyleSheet("background-color: #16213e;")
        sidebar_layout = QVBoxLayout(sidebar)
        sidebar_layout.setContentsMargins(0, 0, 0, 0)
        
        # Header
        header = QFrame()
        header.setStyleSheet("background-color: #0f3460; padding: 10px;")
        header_layout = QVBoxLayout(header)
        self.title_label = QLabel("📡 Meshtastic Monitor")
        self.title_label.setStyleSheet("color: white; font-size: 18px; font-weight: bold;")
        self.status_label = QLabel("⚫ Отключено")
        self.status_label.setStyleSheet("color: #aaa; font-size: 12px;")
        self.update_label = QLabel("Обновление...")
        self.update_label.setStyleSheet("color: #aaa; font-size: 10px;")
        header_layout.addWidget(self.title_label)
        header_layout.addWidget(self.status_label)
        header_layout.addWidget(self.update_label)
        
        # Tabs
        self.tabs = QTabWidget()
        self.tabs.setStyleSheet("""
            QTabWidget::pane { border: 1px solid #0f3460; background: #16213e; }
            QTabBar::tab {
                background: #0f3460; color: #aaa; padding: 8px 12px;
                border: none; border-right: 1px solid #16213e;
            }
            QTabBar::tab:selected { background: #16213e; color: white; }
            QTabBar::tab:hover { background: #1a3a5c; color: white; }
        """)
        
        # Overview tab
        overview_tab = self.create_overview_tab()
        self.tabs.addTab(overview_tab, "📊 Обзор")
        
        # Metrics tab
        metrics_tab = self.create_metrics_tab()
        self.tabs.addTab(metrics_tab, "❤️ Метрики")
        
        # Alerts tab
        alerts_tab = self.create_alerts_tab()
        self.tabs.addTab(alerts_tab, "🔔 Алерты")
        
        # Messages tab
        messages_tab = self.create_messages_tab()
        self.tabs.addTab(messages_tab, "💬 Сообщения")
        
        # Settings tab
        settings_tab = self.create_settings_tab()
        self.tabs.addTab(settings_tab, "⚙️ Настройки")
        
        sidebar_layout.addWidget(header)
        sidebar_layout.addWidget(self.tabs)
        
        # Add to main layout
        main_layout.addWidget(self.map_widget)
        main_layout.addWidget(sidebar)
        
        # Status bar
        self.statusBar = QStatusBar()
        self.setStatusBar(self.statusBar)
        self.statusBar.showMessage("Готов")
        
        # Refresh timer
        self.refresh_timer.start(5000)  # 5 секунд
        
    def create_overview_tab(self) -> QWidget:
        """Создание вкладки обзора"""
        widget = QWidget()
        widget.setStyleSheet("background: #16213e; color: #eee;")
        layout = QVBoxLayout(widget)
        
        # Stats
        stats_layout = QHBoxLayout()
        
        self.device_count_label = QLabel("0")
        self.device_count_label.setStyleSheet("""
            QLabel { font-size: 32px; font-weight: bold; color: #4ecca3; }
        """)
        device_count_widget = QVBoxLayout()
        device_count_widget.addWidget(self.device_count_label)
        device_count_widget.addWidget(QLabel("Устройств"))
        
        self.alert_count_label = QLabel("0")
        self.alert_count_label.setStyleSheet("""
            QLabel { font-size: 32px; font-weight: bold; color: #4ecca3; }
        """)
        alert_count_widget = QVBoxLayout()
        alert_count_widget.addWidget(self.alert_count_label)
        alert_count_widget.addWidget(QLabel("Алертов"))
        
        stats_layout.addLayout(device_count_widget)
        stats_layout.addLayout(alert_count_widget)
        layout.addLayout(stats_layout)
        
        # Devices list
        devices_group = QGroupBox("📍 Устройства")
        devices_group.setStyleSheet("""
            QGroupBox {
                color: #4ecca3; border: 1px solid #0f3460;
                border-radius: 5px; margin-top: 10px; padding-top: 10px;
            }
            QGroupBox::title { subcontrol-origin: margin; left: 10px; }
        """)
        devices_layout = QVBoxLayout(devices_group)
        self.devices_list = QListWidget()
        self.devices_list.setStyleSheet("""
            QListWidget {
                background: #1a1a2e; border: none; color: #eee;
            }
            QListWidget::item {
                padding: 8px; border-bottom: 1px solid #0f3460;
            }
            QListWidget::item:selected { background: #0f3460; }
        """)
        devices_layout.addWidget(self.devices_list)
        layout.addWidget(devices_group)
        
        # Discovery
        discovery_group = QGroupBox("📡 Discovery")
        discovery_group.setStyleSheet("""
            QGroupBox {
                color: #4ecca3; border: 1px solid #0f3460;
                border-radius: 5px; margin-top: 10px; padding-top: 10px;
            }
            QGroupBox::title { subcontrol-origin: margin; left: 10px; }
        """)
        discovery_layout = QVBoxLayout(discovery_group)
        
        btn_layout = QHBoxLayout()
        self.bt_scan_btn = QPushButton("🔵 Bluetooth")
        self.bt_scan_btn.setStyleSheet("""
            QPushButton {
                background: #0f3460; color: white; padding: 8px;
                border: 1px solid #4ecca3; border-radius: 4px;
            }
            QPushButton:hover { background: #1a3a5c; }
        """)
        self.bt_scan_btn.clicked.connect(self.toggle_bluetooth_scan)
        
        self.wifi_scan_btn = QPushButton("📶 WiFi")
        self.wifi_scan_btn.setStyleSheet("""
            QPushButton {
                background: #0f3460; color: white; padding: 8px;
                border: 1px solid #4ecca3; border-radius: 4px;
            }
            QPushButton:hover { background: #1a3a5c; }
        """)
        self.wifi_scan_btn.clicked.connect(self.toggle_wifi_scan)
        
        btn_layout.addWidget(self.bt_scan_btn)
        btn_layout.addWidget(self.wifi_scan_btn)
        discovery_layout.addLayout(btn_layout)
        
        self.discovery_status = QLabel("")
        self.discovery_status.setStyleSheet("color: #aaa; font-size: 11px;")
        discovery_layout.addWidget(self.discovery_status)
        
        layout.addWidget(discovery_group)
        
        return widget
        
    def create_metrics_tab(self) -> QWidget:
        """Создание вкладки метрик"""
        widget = QWidget()
        widget.setStyleSheet("background: #16213e; color: #eee;")
        layout = QVBoxLayout(widget)
        
        self.metrics_list = QListWidget()
        self.metrics_list.setStyleSheet("""
            QListWidget {
                background: #1a1a2e; border: none; color: #eee;
            }
            QListWidget::item { padding: 10px; border-bottom: 1px solid #0f3460; }
        """)
        layout.addWidget(self.metrics_list)
        
        return widget
        
    def create_alerts_tab(self) -> QWidget:
        """Создание вкладки алертов"""
        widget = QWidget()
        widget.setStyleSheet("background: #16213e; color: #eee;")
        layout = QVBoxLayout(widget)
        
        # Header with button
        header_layout = QHBoxLayout()
        header_layout.addWidget(QLabel("🔔 Уведомления"))
        clear_btn = QPushButton("✓ Все прочитано")
        clear_btn.setStyleSheet("""
            QPushButton {
                background: #0f3460; color: white; padding: 5px 10px;
                border: 1px solid #4ecca3; border-radius: 4px;
            }
        """)
        clear_btn.clicked.connect(self.mark_all_alerts_read)
        header_layout.addWidget(clear_btn)
        layout.addLayout(header_layout)
        
        self.alerts_list = QListWidget()
        self.alerts_list.setStyleSheet("""
            QListWidget {
                background: #1a1a2e; border: none; color: #eee;
            }
            QListWidget::item { padding: 8px; border-bottom: 1px solid #0f3460; }
        """)
        layout.addWidget(self.alerts_list)
        
        return widget
        
    def create_messages_tab(self) -> QWidget:
        """Создание вкладки сообщений"""
        widget = QWidget()
        widget.setStyleSheet("background: #16213e; color: #eee;")
        layout = QVBoxLayout(widget)
        
        # Form
        form = QFormLayout()
        
        self.msg_device_id = QLineEdit("1")
        self.msg_device_id.setStyleSheet("""
            QLineEdit {
                background: #1a1a2e; border: 1px solid #0f3460;
                color: white; padding: 5px; border-radius: 4px;
            }
        """)
        form.addRow("Device ID:", self.msg_device_id)
        
        self.msg_from = QLineEdit("!12345678")
        self.msg_from.setStyleSheet("""
            QLineEdit {
                background: #1a1a2e; border: 1px solid #0f3460;
                color: white; padding: 5px; border-radius: 4px;
            }
        """)
        form.addRow("От кого:", self.msg_from)
        
        self.msg_to = QLineEdit("!87654321")
        self.msg_to.setStyleSheet("""
            QLineEdit {
                background: #1a1a2e; border: 1px solid #0f3460;
                color: white; padding: 5px; border-radius: 4px;
            }
        """)
        form.addRow("Кому:", self.msg_to)
        
        self.msg_text = QTextEdit()
        self.msg_text.setMaximumHeight(80)
        self.msg_text.setStyleSheet("""
            QTextEdit {
                background: #1a1a2e; border: 1px solid #0f3460;
                color: white; padding: 5px; border-radius: 4px;
            }
        """)
        form.addRow("Сообщение:", self.msg_text)
        
        layout.addLayout(form)
        
        send_btn = QPushButton("📤 Отправить")
        send_btn.setStyleSheet("""
            QPushButton {
                background: #4ecca3; color: #1a1a2e; font-weight: bold;
                padding: 10px; border: none; border-radius: 4px;
            }
            QPushButton:hover { background: #3db892; }
        """)
        send_btn.clicked.connect(self.send_message)
        layout.addWidget(send_btn)
        
        # Messages history
        layout.addWidget(QLabel("📋 История сообщений"))
        self.messages_list = QListWidget()
        self.messages_list.setStyleSheet("""
            QListWidget {
                background: #1a1a2e; border: none; color: #eee;
            }
            QListWidget::item { padding: 8px; border-bottom: 1px solid #0f3460; }
        """)
        layout.addWidget(self.messages_list)
        
        return widget
        
    def create_settings_tab(self) -> QWidget:
        """Создание вкладки настроек"""
        widget = QWidget()
        widget.setStyleSheet("background: #16213e; color: #eee;")
        layout = QVBoxLayout(widget)
        
        # Server settings
        server_group = QGroupBox("🔌 Подключение к серверу")
        server_group.setStyleSheet("""
            QGroupBox {
                color: #4ecca3; border: 1px solid #0f3460;
                border-radius: 5px; margin-top: 10px; padding-top: 10px;
            }
            QGroupBox::title { subcontrol-origin: margin; left: 10px; }
        """)
        server_layout = QVBoxLayout(server_group)
        
        url_layout = QHBoxLayout()
        self.server_url_input = QLineEdit("http://localhost:8080")
        self.server_url_input.setStyleSheet("""
            QLineEdit {
                background: #1a1a2e; border: 1px solid #0f3460;
                color: white; padding: 8px; border-radius: 4px;
            }
        """)
        url_layout.addWidget(self.server_url_input)
        
        save_url_btn = QPushButton("💾")
        save_url_btn.setMaximumWidth(50)
        save_url_btn.setStyleSheet("""
            QPushButton {
                background: #0f3460; color: white;
                border: 1px solid #4ecca3; border-radius: 4px;
            }
        """)
        save_url_btn.clicked.connect(self.save_server_url)
        url_layout.addWidget(save_url_btn)
        
        server_layout.addLayout(url_layout)
        
        check_btn = QPushButton("✓ Проверить подключение")
        check_btn.setStyleSheet("""
            QPushButton {
                background: #4ecca3; color: #1a1a2e; font-weight: bold;
                padding: 10px; border: none; border-radius: 4px;
            }
            QPushButton:hover { background: #3db892; }
        """)
        check_btn.clicked.connect(self.check_connection)
        server_layout.addWidget(check_btn)
        
        self.connection_status = QLabel("")
        self.connection_status.setStyleSheet("font-size: 12px;")
        server_layout.addWidget(self.connection_status)
        
        layout.addWidget(server_group)
        
        # About
        about_group = QGroupBox("ℹ️ О приложении")
        about_group.setStyleSheet("""
            QGroupBox {
                color: #4ecca3; border: 1px solid #0f3460;
                border-radius: 5px; margin-top: 10px; padding-top: 10px;
            }
            QGroupBox::title { subcontrol-origin: margin; left: 10px; }
        """)
        about_layout = QVBoxLayout(about_group)
        about_layout.addWidget(QLabel("<b>Meshtastic Monitor Desktop</b>"))
        about_layout.addWidget(QLabel("Версия: 1.0.0"))
        about_layout.addWidget(QLabel("Платформа: Python + PySide6"))
        layout.addWidget(about_group)
        
        return widget

    def check_connection(self):
        """Проверка подключения к серверу"""
        connected = self.client.check_connection()
        if connected:
            self.status_label.setText("🟢 Подключено")
            self.status_label.setStyleSheet("color: #4ecca3;")
            self.connection_status.setText("✅ Сервер доступен")
            self.connection_status.setStyleSheet("color: #4ecca3;")
            self.refresh_data()
        else:
            self.status_label.setText("🔴 Отключено")
            self.status_label.setStyleSheet("color: #dc3545;")
            self.connection_status.setText("❌ Сервер недоступен")
            self.connection_status.setStyleSheet("color: #dc3545;")

    def save_server_url(self):
        """Сохранение URL сервера"""
        url = self.server_url_input.text()
        self.client = MeshServerClient(url)
        self.connection_status.setText(f"URL сохранен: {url}")
        self.check_connection()

    def refresh_data(self):
        """Обновление данных"""
        self.devices = self.client.get_devices()
        self.metrics = self.client.get_metrics()
        self.alerts = self.client.get_unread_alerts()
        
        # Update UI
        self.update_devices_list()
        self.update_metrics_list()
        self.update_alerts_list()
        self.update_counts()
        
        self.update_label.setText(f"Обновлено: {datetime.now().strftime('%H:%M:%S')}")

    def update_devices_list(self):
        """Обновление списка устройств"""
        self.devices_list.clear()
        for device in self.devices:
            item_text = f"{device.name or device.node_id}"
            status = "🟢" if device.is_online else "🔴"
            coords = f"{device.latitude:.4f}, {device.longitude:.4f}"
            
            item = QListWidgetItem(f"{status} {item_text}\n📍 {coords}")
            item.setForeground(QBrush(QColor("#4ecca3" if device.is_online else "#dc3545")))
            self.devices_list.addItem(item)

    def update_metrics_list(self):
        """Обновление списка метрик"""
        self.metrics_list.clear()
        for metric in self.metrics:
            item = QListWidgetItem(
                f"Device #{metric.device_id}\n"
                f"❤️ {metric.heart_rate} bpm | "
                f"💨 {metric.co2} ppm | "
                f"🌡️ {metric.temp}°C | "
                f"💧 {metric.humidity}%"
            )
            self.metrics_list.addItem(item)

    def update_alerts_list(self):
        """Обновление списка алертов"""
        self.alerts_list.clear()
        for alert in self.alerts:
            color = {"critical": "#dc3545", "warning": "#ffc107", "info": "#0dcaf0"}.get(
                alert.severity, "#aaa"
            )
            item = QListWidgetItem(f"⚠️ {alert.type}\n{alert.message}")
            item.setForeground(QBrush(QColor(color)))
            
            # Add button to mark as read
            btn = QPushButton("✓")
            btn.setMaximumWidth(30)
            btn.clicked.connect(lambda checked, aid=alert.id: self.mark_alert_read(aid))
            
            self.alerts_list.addItem(item)
            self.alerts_list.setItemWidget(item, btn)

    def update_counts(self):
        """Обновление счетчиков"""
        self.device_count_label.setText(str(len(self.devices)))
        self.alert_count_label.setText(str(len(self.alerts)))

    def toggle_bluetooth_scan(self):
        """Переключение Bluetooth сканирования"""
        status = self.client.get_discovery_status()
        if status.get("bluetooth_scanning"):
            self.client.stop_bluetooth_scan()
            self.discovery_status.setText("Bluetooth scan stopped")
        else:
            self.client.start_bluetooth_scan()
            self.discovery_status.setText("Bluetooth scanning...")

    def toggle_wifi_scan(self):
        """Переключение WiFi сканирования"""
        status = self.client.get_discovery_status()
        if status.get("wifi_scanning"):
            self.client.stop_wifi_scan()
            self.discovery_status.setText("WiFi scan stopped")
        else:
            self.client.start_wifi_scan()
            self.discovery_status.setText("WiFi scanning...")

    def mark_alert_read(self, alert_id: int):
        """Пометить алерт как прочитанный"""
        if self.client.mark_alert_read(alert_id):
            self.refresh_data()

    def mark_all_alerts_read(self):
        """Пометить все алерты как прочитанные"""
        if self.client.mark_all_alerts_read():
            self.refresh_data()

    def send_message(self):
        """Отправить сообщение"""
        try:
            device_id = int(self.msg_device_id.text())
            from_node = self.msg_from.text()
            to_node = self.msg_to.text()
            text = self.msg_text.toPlainText()
            
            if not text:
                QMessageBox.warning(self, "Ошибка", "Введите текст сообщения")
                return
            
            if self.client.send_message(device_id, from_node, to_node, text):
                QMessageBox.information(self, "Успех", "Сообщение отправлено!")
                self.msg_text.clear()
                self.messages = self.client.get_messages(device_id)
                self.update_messages_list()
            else:
                QMessageBox.warning(self, "Ошибка", "Не удалось отправить сообщение")
        except ValueError:
            QMessageBox.warning(self, "Ошибка", "Неверный Device ID")

    def update_messages_list(self):
        """Обновление списка сообщений"""
        self.messages_list.clear()
        for msg in self.messages:
            direction = "→" if msg.direction == "outbound" else "←"
            color = "#4ecca3" if msg.direction == "outbound" else "#0dcaf0"
            item = QListWidgetItem(f"{direction} {msg.from_node} → {msg.to_node}\n{msg.text}")
            item.setForeground(QBrush(QColor(color)))
            self.messages_list.addItem(item)


def main():
    app = QApplication(sys.argv)
    
    # Set dark palette
    app.setStyle("Fusion")
    
    window = MainWindow()
    window.show()
    
    sys.exit(app.exec())


if __name__ == "__main__":
    main()
