"""
Виджеты для основного окна приложения
"""
from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QTableWidget, QTableWidgetItem,
    QPushButton, QLabel, QTextEdit, QLineEdit, QComboBox, QSpinBox,
    QDoubleSpinBox, QTabWidget, QGroupBox, QHeaderView, QMessageBox,
    QFileDialog, QCheckBox, QDialog
)
from PySide6.QtCore import Qt, QThread, Signal
from PySide6.QtGui import QFont

from typing import List, Dict, Optional
from api_client import MeshAPIClient


class DevicesTab(QWidget):
    """Вкладка устройств"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        self.refresh()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("📱 Устройства"))
        header.addStretch()
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        header.addWidget(btn_refresh)
        
        btn_add = QPushButton("➕ Добавить")
        btn_add.clicked.connect(self._add_device)
        header.addWidget(btn_add)
        
        layout.addLayout(header)
        
        # Таблица устройств
        self.table = QTableWidget()
        self.table.setColumnCount(6)
        self.table.setHorizontalHeaderLabels([
            "ID", "Node ID", "Название", "Широта", "Долгота", "Высота"
        ])
        self.table.horizontalHeader().setSectionResizeMode(QHeaderView.Stretch)
        layout.addWidget(self.table)
        
    def refresh(self):
        """Обновление списка устройств"""
        devices = self.api.get_devices()
        
        self.table.setRowCount(0)
        for device in devices:
            row = self.table.rowCount()
            self.table.insertRow(row)
            self.table.setItem(row, 0, QTableWidgetItem(str(device.get('id', ''))))
            self.table.setItem(row, 1, QTableWidgetItem(device.get('node_id', '')))
            self.table.setItem(row, 2, QTableWidgetItem(device.get('name', '')))
            self.table.setItem(row, 3, QTableWidgetItem(str(device.get('latitude', ''))))
            self.table.setItem(row, 4, QTableWidgetItem(str(device.get('longitude', ''))))
            self.table.setItem(row, 5, QTableWidgetItem(str(device.get('altitude', ''))))
            
    def _add_device(self):
        """Диалог добавления устройства"""
        dialog = AddDeviceDialog(self)
        if dialog.exec() and dialog.device_data:
            self.api.create_device(dialog.device_data)
            self.refresh()


class AddDeviceDialog(QDialog):
    """Диалог добавления устройства"""
    
    def __init__(self, parent=None):
        super().__init__(parent)
        self.device_data = None
        self._setup_ui()
        
    def _setup_ui(self):
        self.setWindowTitle("Добавить устройство")
        self.setMinimumWidth(400)
        
        layout = QVBoxLayout(self)
        
        # Node ID
        layout.addWidget(QLabel("Node ID:"))
        self.node_id_edit = QLineEdit()
        self.node_id_edit.setPlaceholderText("!12345678")
        layout.addWidget(self.node_id_edit)
        
        # Название
        layout.addWidget(QLabel("Название:"))
        self.name_edit = QLineEdit()
        self.name_edit.setPlaceholderText("Device 1")
        layout.addWidget(self.name_edit)
        
        # Координаты
        coords_layout = QHBoxLayout()
        
        coords_layout.addWidget(QLabel("Широта:"))
        self.lat_edit = QDoubleSpinBox()
        self.lat_edit.setRange(-90, 90)
        self.lat_edit.setValue(55.7558)
        coords_layout.addWidget(self.lat_edit)
        
        coords_layout.addWidget(QLabel("Долгота:"))
        self.lon_edit = QDoubleSpinBox()
        self.lon_edit.setRange(-180, 180)
        self.lon_edit.setValue(37.6173)
        coords_layout.addWidget(self.lon_edit)
        
        layout.addLayout(coords_layout)
        
        # Высота
        layout.addWidget(QLabel("Высота (м):"))
        self.alt_edit = QSpinBox()
        self.alt_edit.setRange(-1000, 10000)
        layout.addWidget(self.alt_edit)
        
        # Кнопки
        btn_layout = QHBoxLayout()
        btn_layout.addStretch()
        
        btn_cancel = QPushButton("Отмена")
        btn_cancel.clicked.connect(self.reject)
        btn_layout.addWidget(btn_cancel)
        
        btn_ok = QPushButton("OK")
        btn_ok.clicked.connect(self._accept)
        btn_layout.addWidget(btn_ok)
        
        layout.addLayout(btn_layout)
        
    def _accept(self):
        self.device_data = {
            'node_id': self.node_id_edit.text(),
            'name': self.name_edit.text(),
            'latitude': self.lat_edit.value(),
            'longitude': self.lon_edit.value(),
            'altitude': self.alt_edit.value()
        }
        self.accept()


class MetricsTab(QWidget):
    """Вкладка метрик"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        self.refresh()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("📊 Метрики"))
        header.addStretch()
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        header.addWidget(btn_refresh)
        
        layout.addLayout(header)
        
        # Таблица метрик
        self.table = QTableWidget()
        self.table.setColumnCount(7)
        self.table.setHorizontalHeaderLabels([
            "ID", "Устройство", "Пульс", "CO2", "Температура", "Влажность", "Время"
        ])
        self.table.horizontalHeader().setSectionResizeMode(QHeaderView.Stretch)
        layout.addWidget(self.table)
        
    def refresh(self):
        """Обновление метрик"""
        metrics = self.api.get_metrics()
        
        self.table.setRowCount(0)
        for metric in metrics:
            row = self.table.rowCount()
            self.table.insertRow(row)
            self.table.setItem(row, 0, QTableWidgetItem(str(metric.get('id', ''))))
            self.table.setItem(row, 1, QTableWidgetItem(str(metric.get('device_id', ''))))
            self.table.setItem(row, 2, QTableWidgetItem(str(metric.get('heart_rate', ''))))
            self.table.setItem(row, 3, QTableWidgetItem(str(metric.get('co2', ''))))
            self.table.setItem(row, 4, QTableWidgetItem(str(metric.get('temp', ''))))
            self.table.setItem(row, 5, QTableWidgetItem(str(metric.get('humidity', ''))))
            self.table.setItem(row, 6, QTableWidgetItem(metric.get('timestamp', '')))
            
    def update_from_ws(self, data: dict):
        """Обновление из WebSocket"""
        self.refresh()


class AlertsTab(QWidget):
    """Вкладка алертов"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        self.refresh()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("🔔 Алерты"))
        header.addStretch()
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        header.addWidget(btn_refresh)
        
        btn_mark_all = QPushButton("✅ Все прочитано")
        btn_mark_all.clicked.connect(self._mark_all_read)
        header.addWidget(btn_mark_all)
        
        layout.addLayout(header)
        
        # Таблица алертов
        self.table = QTableWidget()
        self.table.setColumnCount(6)
        self.table.setHorizontalHeaderLabels([
            "ID", "Устройство", "Тип", "Сообщение", "Важность", "Прочитано"
        ])
        self.table.horizontalHeader().setSectionResizeMode(QHeaderView.Stretch)
        self.table.cellDoubleClicked.connect(self._mark_read)
        layout.addWidget(self.table)
        
    def refresh(self):
        """Обновление алертов"""
        alerts = self.api.get_alerts(unread_only=False)
        
        self.table.setRowCount(0)
        for alert in alerts:
            row = self.table.rowCount()
            self.table.insertRow(row)
            self.table.setItem(row, 0, QTableWidgetItem(str(alert.get('id', ''))))
            self.table.setItem(row, 1, QTableWidgetItem(str(alert.get('device_id', ''))))
            self.table.setItem(row, 2, QTableWidgetItem(alert.get('type', '')))
            self.table.setItem(row, 3, QTableWidgetItem(alert.get('message', '')))
            
            severity_item = QTableWidgetItem(alert.get('severity', ''))
            if alert.get('severity') == 'critical':
                severity_item.setBackground(Qt.red)
            elif alert.get('severity') == 'warning':
                severity_item.setBackground(Qt.yellow)
            self.table.setItem(row, 4, severity_item)
            
            read_item = QTableWidgetItem("✅" if alert.get('read') else "❌")
            read_item.setFlags(read_item.flags() & ~Qt.ItemIsEditable)
            self.table.setItem(row, 5, read_item)
            
    def _mark_read(self, row: int, col: int):
        """Пометить алерт как прочитанный (двойной клик)"""
        alert_id = int(self.table.item(row, 0).text())
        self.api.mark_alert_read(alert_id)
        self.refresh()
        
    def _mark_all_read(self):
        """Пометить все алерты как прочитанные"""
        self.api.mark_all_alerts_read()
        self.refresh()
        
    def add_alert_from_ws(self, data: dict):
        """Добавление алерта из WebSocket"""
        self.refresh()


class MessagesTab(QWidget):
    """Вкладка сообщений"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        self.refresh()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("💬 Сообщения"))
        header.addStretch()
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        header.addWidget(btn_refresh)
        
        layout.addLayout(header)
        
        # Форма отправки
        form_group = QGroupBox("Отправить сообщение")
        form_layout = QVBoxLayout(form_group)
        
        # Device ID
        device_layout = QHBoxLayout()
        device_layout.addWidget(QLabel("Устройство ID:"))
        self.device_id_edit = QSpinBox()
        self.device_id_edit.setRange(1, 10000)
        device_layout.addWidget(self.device_id_edit)
        device_layout.addStretch()
        form_layout.addLayout(device_layout)
        
        # From Node
        node_layout = QHBoxLayout()
        node_layout.addWidget(QLabel("From Node:"))
        self.from_node_edit = QLineEdit()
        self.from_node_edit.setPlaceholderText("!12345678")
        node_layout.addWidget(self.from_node_edit)
        node_layout.addWidget(QLabel("To Node:"))
        self.to_node_edit = QLineEdit()
        self.to_node_edit.setPlaceholderText("!87654321")
        node_layout.addWidget(self.to_node_edit)
        form_layout.addLayout(node_layout)
        
        # Текст сообщения
        form_layout.addWidget(QLabel("Сообщение:"))
        self.message_edit = QTextEdit()
        self.message_edit.setMaximumHeight(80)
        form_layout.addWidget(self.message_edit)
        
        # Кнопка отправки
        btn_send = QPushButton("📤 Отправить")
        btn_send.clicked.connect(self._send_message)
        form_layout.addWidget(btn_send)
        
        layout.addWidget(form_group)
        
        # Таблица сообщений
        self.table = QTableWidget()
        self.table.setColumnCount(6)
        self.table.setHorizontalHeaderLabels([
            "ID", "Устройство", "From", "To", "Текст", "Направление"
        ])
        self.table.horizontalHeader().setSectionResizeMode(QHeaderView.Stretch)
        layout.addWidget(self.table)
        
    def refresh(self):
        """Обновление сообщений"""
        messages = self.api.get_messages()
        
        self.table.setRowCount(0)
        for msg in messages:
            row = self.table.rowCount()
            self.table.insertRow(row)
            self.table.setItem(row, 0, QTableWidgetItem(str(msg.get('id', ''))))
            self.table.setItem(row, 1, QTableWidgetItem(str(msg.get('device_id', ''))))
            self.table.setItem(row, 2, QTableWidgetItem(msg.get('from_node', '')))
            self.table.setItem(row, 3, QTableWidgetItem(msg.get('to_node', '')))
            self.table.setItem(row, 4, QTableWidgetItem(msg.get('text', '')))
            self.table.setItem(row, 5, QTableWidgetItem(msg.get('direction', '')))
            
    def _send_message(self):
        """Отправка сообщения"""
        message_data = {
            'device_id': self.device_id_edit.value(),
            'from_node': self.from_node_edit.text(),
            'to_node': self.to_node_edit.text(),
            'text': self.message_edit.toPlainText(),
            'direction': 'outbound'
        }
        
        result = self.api.send_message(message_data)
        if result:
            QMessageBox.information(self, "Успех", "Сообщение отправлено!")
            self.message_edit.clear()
            self.refresh()
        else:
            QMessageBox.critical(self, "Ошибка", "Не удалось отправить сообщение")
            
    def add_message_from_ws(self, data: dict):
        """Добавление сообщения из WebSocket"""
        self.refresh()


class MapTab(QWidget):
    """Вкладка карты (упрощенная - таблица координат)"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        self.refresh()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("🗺️ Карта расположений"))
        header.addStretch()
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        header.addWidget(btn_refresh)
        
        layout.addLayout(header)
        
        # Информация
        info_label = QLabel("💡 Для полноценной карты интегрируйте Яндекс.Карты или OpenStreetMap")
        info_label.setStyleSheet("color: gray; font-style: italic;")
        layout.addWidget(info_label)
        
        # Таблица координат
        self.table = QTableWidget()
        self.table.setColumnCount(6)
        self.table.setHorizontalHeaderLabels([
            "Node ID", "Название", "Широта", "Долгота", "Высота", "Последний раз"
        ])
        self.table.horizontalHeader().setSectionResizeMode(QHeaderView.Stretch)
        layout.addWidget(self.table)
        
    def refresh(self):
        """Обновление данных карты"""
        map_data = self.api.get_map_data()
        
        self.table.setRowCount(0)
        for device in map_data:
            row = self.table.rowCount()
            self.table.insertRow(row)
            self.table.setItem(row, 0, QTableWidgetItem(device.get('node_id', '')))
            self.table.setItem(row, 1, QTableWidgetItem(device.get('name', '')))
            self.table.setItem(row, 2, QTableWidgetItem(str(device.get('latitude', ''))))
            self.table.setItem(row, 3, QTableWidgetItem(str(device.get('longitude', ''))))
            self.table.setItem(row, 4, QTableWidgetItem(str(device.get('altitude', ''))))
            self.table.setItem(row, 5, QTableWidgetItem(device.get('last_seen', '')))


class DiscoveryTab(QWidget):
    """Вкладка Discovery - обнаружение устройств"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("📡 Discovery - Обнаружение устройств"))
        header.addStretch()
        layout.addLayout(header)
        
        # Кнопки управления
        btn_layout = QHBoxLayout()
        
        self.btn_bluetooth_scan = QPushButton("🔵 Bluetooth Старт")
        self.btn_bluetooth_scan.clicked.connect(self._toggle_bluetooth_scan)
        btn_layout.addWidget(self.btn_bluetooth_scan)
        
        self.btn_wifi_scan = QPushButton("🟢 WiFi Старт")
        self.btn_wifi_scan.clicked.connect(self._toggle_wifi_scan)
        btn_layout.addWidget(self.btn_wifi_scan)
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        btn_layout.addWidget(btn_refresh)
        
        btn_clear = QPushButton("🗑️ Очистить")
        btn_clear.clicked.connect(self._clear_devices)
        btn_layout.addWidget(btn_clear)
        
        layout.addLayout(btn_layout)
        
        # Статус
        self.status_label = QLabel("Статус: Неизвестно")
        layout.addWidget(self.status_label)
        
        # Вкладки для типов устройств
        self.device_tabs = QTabWidget()
        
        # Bluetooth устройства
        self.bt_table = self._create_device_table()
        self.device_tabs.addTab(self.bt_table, "Bluetooth")
        
        # WiFi устройства
        self.wifi_table = self._create_device_table()
        self.device_tabs.addTab(self.wifi_table, "WiFi")
        
        # Meshtastic устройства
        self.mesh_table = self._create_device_table()
        self.device_tabs.addTab(self.mesh_table, "Meshtastic")
        
        layout.addWidget(self.device_tabs)
        
        # Сетевая информация
        network_group = QGroupBox("Сетевая информация")
        network_layout = QVBoxLayout(network_group)
        self.network_info_label = QLabel("Нажмите 'Обновить' для загрузки")
        network_layout.addWidget(self.network_info_label)
        layout.addWidget(network_group)
        
        self.bluetooth_scanning = False
        self.wifi_scanning = False
        
    def _create_device_table(self) -> QTableWidget:
        """Создание таблицы устройств"""
        table = QTableWidget()
        table.setColumnCount(5)
        table.setHorizontalHeaderLabels([
            "Адрес", "Название", "Тип", "RSSI", "Последний раз"
        ])
        table.horizontalHeader().setSectionResizeMode(QHeaderView.Stretch)
        return table
        
    def refresh(self):
        """Обновление данных"""
        # Статус
        status = self.api.get_discovery_status()
        self.bluetooth_scanning = status.get('bluetooth_scanning', False)
        self.wifi_scanning = status.get('wifi_scanning', False)
        self._update_scan_buttons()
        
        self.status_label.setText(
            f"Статус: BT={('🔵 Сканирование' if self.bluetooth_scanning else '⚫ Остановлено')}, "
            f"WiFi={('🟢 Сканирование' if self.wifi_scanning else '⚫ Остановлено')}, "
            f"Найдено: {status.get('devices_found', 0)}"
        )
        
        # Bluetooth устройства
        bt_devices = self.api.get_bluetooth_devices()
        self._fill_device_table(self.bt_table, bt_devices)
        
        # WiFi устройства
        wifi_devices = self.api.get_wifi_devices()
        self._fill_device_table(self.wifi_table, wifi_devices)
        
        # Meshtastic устройства
        mesh_devices = self.api.get_meshtastic_devices()
        self._fill_device_table(self.mesh_table, mesh_devices)
        
        # Сетевая информация
        network_info = self.api.get_network_info()
        if network_info:
            self.network_info_label.setText(str(network_info))
            
    def _fill_device_table(self, table: QTableWidget, devices: List[Dict]):
        """Заполнение таблицы устройств"""
        table.setRowCount(0)
        for device in devices:
            row = table.rowCount()
            table.insertRow(row)
            table.setItem(row, 0, QTableWidgetItem(device.get('address', '')))
            table.setItem(row, 1, QTableWidgetItem(device.get('name', '')))
            table.setItem(row, 2, QTableWidgetItem(device.get('type', '')))
            table.setItem(row, 3, QTableWidgetItem(str(device.get('rssi', ''))))
            table.setItem(row, 4, QTableWidgetItem(device.get('last_seen', '')))
            
    def _toggle_bluetooth_scan(self):
        """Переключение Bluetooth сканирования"""
        if self.bluetooth_scanning:
            self.api.stop_bluetooth_scan()
        else:
            self.api.start_bluetooth_scan()
        self.refresh()
        
    def _toggle_wifi_scan(self):
        """Переключение WiFi сканирования"""
        if self.wifi_scanning:
            self.api.stop_wifi_scan()
        else:
            self.api.start_wifi_scan()
        self.refresh()
        
    def _update_scan_buttons(self):
        """Обновление кнопок сканирования"""
        if self.bluetooth_scanning:
            self.btn_bluetooth_scan.setText("🔴 Bluetooth Стоп")
        else:
            self.btn_bluetooth_scan.setText("🔵 Bluetooth Старт")
            
        if self.wifi_scanning:
            self.btn_wifi_scan.setText("🔴 WiFi Стоп")
        else:
            self.btn_wifi_scan.setText("🟢 WiFi Старт")
            
    def _clear_devices(self):
        """Очистка списка устройств"""
        self.api.clear_discovery()
        self.refresh()


class SimulatorTab(QWidget):
    """Вкладка симулятора"""
    
    def __init__(self, api: MeshAPIClient):
        super().__init__()
        self.api = api
        self._setup_ui()
        self.refresh()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("🧪 Симулятор носимого устройства"))
        header.addStretch()
        
        btn_refresh = QPushButton("🔄 Обновить")
        btn_refresh.clicked.connect(lambda: self.refresh())
        header.addWidget(btn_refresh)
        
        layout.addLayout(header)
        
        # Статус
        self.status_group = QGroupBox("Статус симулятора")
        status_layout = QVBoxLayout(self.status_group)
        self.status_label = QLabel("Загрузка...")
        status_layout.addWidget(self.status_label)
        layout.addWidget(self.status_group)
        
        # Управление
        control_group = QGroupBox("Управление")
        control_layout = QVBoxLayout(control_group)
        
        btn_layout = QHBoxLayout()
        
        btn_start = QPushButton("▶️ Запустить")
        btn_start.clicked.connect(self._start_simulator)
        btn_layout.addWidget(btn_start)
        
        btn_stop = QPushButton("⏹️ Остановить")
        btn_stop.clicked.connect(self._stop_simulator)
        btn_layout.addWidget(btn_stop)
        
        control_layout.addLayout(btn_layout)
        layout.addWidget(control_group)
        
        # События
        event_group = QGroupBox("Генерация событий")
        event_layout = QVBoxLayout(event_group)
        
        btn_high_hr = QPushButton("❤️ Высокий пульс (140+)")
        btn_high_hr.clicked.connect(lambda: self._trigger_event('high_heart_rate'))
        event_layout.addWidget(btn_high_hr)
        
        btn_low_battery = QPushButton("🔋 Низкий заряд")
        btn_low_battery.clicked.connect(lambda: self._trigger_event('low_battery'))
        event_layout.addWidget(btn_low_battery)
        
        btn_signal_lost = QPushButton("📡 Потеря сигнала")
        btn_signal_lost.clicked.connect(lambda: self._trigger_event('signal_lost'))
        event_layout.addWidget(btn_signal_lost)
        
        layout.addWidget(event_group)
        
        layout.addStretch()
        
    def refresh(self):
        """Обновление статуса"""
        status = self.api.get_simulator_status()
        
        running = status.get('running', False)
        self.status_label.setText(
            f"Статус: {'✅ Запущен' if running else '⏹️ Остановлен'}\n"
            f"Последнее обновление: {status.get('last_update', 'N/A')}"
        )
        
    def _start_simulator(self):
        """Запуск симулятора"""
        if self.api.start_simulator():
            QMessageBox.information(self, "Успех", "Симулятор запущен")
            self.refresh()
        else:
            QMessageBox.critical(self, "Ошибка", "Не удалось запустить симулятор")
            
    def _stop_simulator(self):
        """Остановка симулятора"""
        if self.api.stop_simulator():
            QMessageBox.information(self, "Успех", "Симулятор остановлен")
            self.refresh()
        else:
            QMessageBox.critical(self, "Ошибка", "Не удалось остановить симулятор")
            
    def _trigger_event(self, event_type: str):
        """Генерация события"""
        if self.api.trigger_simulator_event(event_type):
            QMessageBox.information(self, "Успех", f"Событие '{event_type}' создано")
        else:
            QMessageBox.critical(self, "Ошибка", "Не удалось создать событие")


class SettingsTab(QWidget):
    """Вкладка настроек"""
    
    def __init__(self, main_window):
        super().__init__()
        self.main_window = main_window
        self._setup_ui()
        
    def _setup_ui(self):
        layout = QVBoxLayout(self)
        
        # Заголовок
        header = QHBoxLayout()
        header.addWidget(QLabel("⚙️ Настройки"))
        header.addStretch()
        layout.addLayout(header)
        
        # Настройки сервера
        server_group = QGroupBox("Подключение к серверу")
        server_layout = QVBoxLayout(server_group)
        
        server_layout.addWidget(QLabel("URL сервера:"))
        self.server_url_edit = QLineEdit()
        self.server_url_edit.setText(self.main_window.server_url)
        server_layout.addWidget(self.server_url_edit)
        
        btn_save = QPushButton("💾 Сохранить и переподключить")
        btn_save.clicked.connect(self._save_settings)
        server_layout.addWidget(btn_save)
        
        layout.addWidget(server_group)
        
        # WebSocket
        ws_group = QGroupBox("WebSocket")
        ws_layout = QVBoxLayout(ws_group)
        
        self.ws_status_label = QLabel("Статус: Не подключен")
        ws_layout.addWidget(self.ws_status_label)
        
        btn_connect = QPushButton("🔌 Подключить WebSocket")
        btn_connect.clicked.connect(self._connect_ws)
        ws_layout.addWidget(btn_connect)
        
        btn_disconnect = QPushButton("✖️ Отключить WebSocket")
        btn_disconnect.clicked.connect(self._disconnect_ws)
        ws_layout.addWidget(btn_disconnect)
        
        layout.addWidget(ws_group)
        
        # О программе
        about_group = QGroupBox("О программе")
        about_layout = QVBoxLayout(about_group)
        
        about_label = QLabel(
            "<b>Mesh Server Client</b><br>"
            "Версия: 1.0.0<br>"
            "Desktop клиент для mesh-server<br>"
            "Python + PySide6"
        )
        about_layout.addWidget(about_label)
        
        layout.addWidget(about_group)
        
        layout.addStretch()
        
    def _save_settings(self):
        """Сохранение настроек"""
        new_url = self.server_url_edit.text()
        if new_url != self.main_window.server_url:
            self.main_window.server_url = new_url
            self.main_window.api = MeshAPIClient(new_url)
            self.main_window.disconnect_websocket()
            
            ws_url = new_url.replace("http", "ws")
            self.main_window.ws_client = MeshWebSocketClient(ws_url)
            self.main_window.connect_websocket()
            
            QMessageBox.information(self, "Успех", f"Подключено к {new_url}")
            
    def _connect_ws(self):
        """Подключение к WebSocket"""
        self.main_window.connect_websocket()
        self.ws_status_label.setText("Статус: Подключение...")

    def _disconnect_ws(self):
        """Отключение от WebSocket"""
        self.main_window.disconnect_websocket()
        self.ws_status_label.setText("Статус: Отключен")
