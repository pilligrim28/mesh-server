"""
Mesh Server Client - Desktop приложение для работы с mesh-server
"""
import sys
import os

from PySide6.QtWidgets import QApplication, QMainWindow, QTabWidget, QWidget, QVBoxLayout, QHBoxLayout
from PySide6.QtCore import Qt, QUrl, QTimer, Signal
from PySide6.QtGui import QIcon

from api_client import MeshAPIClient
from ws_client import MeshWebSocketClient
from widgets import (
    DevicesTab, MetricsTab, AlertsTab, MessagesTab, 
    MapTab, DiscoveryTab, SimulatorTab, SettingsTab
)


class MeshServerClient(QMainWindow):
    """Главное окно приложения"""
    
    # Сигналы для обновления UI из WebSocket
    metrics_updated = Signal(dict)
    new_alert = Signal(dict)
    new_message = Signal(dict)
    device_updated = Signal(dict)
    
    def __init__(self, server_url: str = "http://localhost:8080"):
        super().__init__()
        
        self.server_url = server_url
        self.api = MeshAPIClient(server_url)
        self.ws_client = MeshWebSocketClient(server_url)
        
        self._setup_ui()
        self._connect_signals()
        self._setup_timers()
        
    def _setup_ui(self):
        """Настройка пользовательского интерфейса"""
        self.setWindowTitle("Mesh Server Client")
        self.setMinimumSize(1200, 800)
        
        # Центральная панель с вкладками
        central_widget = QWidget()
        self.setCentralWidget(central_widget)
        layout = QVBoxLayout(central_widget)
        layout.setContentsMargins(0, 0, 0, 0)
        
        # Вкладки
        self.tabs = QTabWidget()
        self.tabs.setTabPosition(QTabWidget.North)
        layout.addWidget(self.tabs)
        
        # Добавление вкладок
        self.devices_tab = DevicesTab(self.api)
        self.metrics_tab = MetricsTab(self.api)
        self.alerts_tab = AlertsTab(self.api)
        self.messages_tab = MessagesTab(self.api)
        self.map_tab = MapTab(self.api)
        self.discovery_tab = DiscoveryTab(self.api)
        self.simulator_tab = SimulatorTab(self.api)
        self.settings_tab = SettingsTab(self)
        
        self.tabs.addTab(self.devices_tab, "📱 Устройства")
        self.tabs.addTab(self.metrics_tab, "📊 Метрики")
        self.tabs.addTab(self.alerts_tab, "🔔 Алерты")
        self.tabs.addTab(self.messages_tab, "💬 Сообщения")
        self.tabs.addTab(self.map_tab, "🗺️ Карта")
        self.tabs.addTab(self.discovery_tab, "📡 Discovery")
        self.tabs.addTab(self.simulator_tab, "🧪 Симулятор")
        self.tabs.addTab(self.settings_tab, "⚙️ Настройки")
        
        # Статус бар
        self.statusBar().showMessage("Готов к работе")
        
    def _connect_signals(self):
        """Подключение сигналов WebSocket"""
        # WebSocket события
        self.ws_client.metrics_received.connect(self._on_metrics_update)
        self.ws_client.alert_received.connect(self._on_new_alert)
        self.ws_client.message_received.connect(self._on_new_message)
        
        # Сигналы приложения
        self.metrics_updated.connect(self._handle_metrics_update)
        self.new_alert.connect(self._handle_new_alert)
        self.new_message.connect(self._handle_new_message)
        
    def _setup_timers(self):
        """Настройка таймеров автообновления"""
        # Автообновление данных каждые 10 секунд
        self.refresh_timer = QTimer()
        self.refresh_timer.timeout.connect(self._refresh_data)
        self.refresh_timer.start(10000)  # 10 секунд
        
    def _refresh_data(self):
        """Периодическое обновление данных"""
        current_tab = self.tabs.currentIndex()
        
        # Обновляем данные в зависимости от активной вкладки
        if current_tab == 0:  # Устройства
            self.devices_tab.refresh()
        elif current_tab == 1:  # Метрики
            self.metrics_tab.refresh()
        elif current_tab == 2:  # Алерты
            self.alerts_tab.refresh()
        elif current_tab == 4:  # Карта
            self.map_tab.refresh()
            
    def _on_metrics_update(self, data: dict):
        """Обработка обновления метрик из WebSocket"""
        self.metrics_updated.emit(data)
        
    def _on_new_alert(self, data: dict):
        """Обработка нового алерта из WebSocket"""
        self.new_alert.emit(data)
        
    def _on_new_message(self, data: dict):
        """Обработка нового сообщения из WebSocket"""
        self.new_message.emit(data)
        
    def _handle_metrics_update(self, data: dict):
        """Отображение обновленных метрик"""
        self.metrics_tab.update_from_ws(data)
        self.statusBar().showMessage(f"📊 Метрики обновлены: {data.get('device_name', 'N/A')}")
        
    def _handle_new_alert(self, data: dict):
        """Отображение нового алерта"""
        self.alerts_tab.add_alert_from_ws(data)
        severity = data.get('severity', 'info')
        self.statusBar().showMessage(f"🔔 Новый алерт: {severity}")
        
    def _handle_new_message(self, data: dict):
        """Отображение нового сообщения"""
        self.messages_tab.add_message_from_ws(data)
        self.statusBar().showMessage(f"💬 Новое сообщение")
        
    def connect_websocket(self):
        """Подключение к WebSocket"""
        ws_url = self.server_url.replace("http", "ws")
        self.ws_client.connect(ws_url)
        
    def disconnect_websocket(self):
        """Отключение от WebSocket"""
        self.ws_client.disconnect()
        
    def closeEvent(self, event):
        """Обработка закрытия приложения"""
        self.disconnect_websocket()
        self.refresh_timer.stop()
        event.accept()


def main():
    """Точка входа приложения"""
    # Чтение сервера из аргументов или env
    server_url = "http://localhost:8080"
    if len(sys.argv) > 1:
        server_url = sys.argv[1]
    
    app = QApplication(sys.argv)
    app.setApplicationName("Mesh Server Client")
    
    # Применение стиля
    app.setStyle("Fusion")
    
    window = MeshServerClient(server_url)
    window.show()
    
    # Подключение к WebSocket после показа окна
    QTimer.singleShot(1000, window.connect_websocket)
    
    sys.exit(app.exec())


if __name__ == "__main__":
    main()
