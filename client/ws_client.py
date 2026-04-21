"""
WebSocket клиент для realtime обновлений от mesh-server
"""
import json
import threading
from typing import Optional, Callable

from websocket import WebSocketApp, WebSocketConnectionClosedException
from PySide6.QtCore import QObject, Signal


class MeshWebSocketClient(QObject):
    """Клиент для подключения к WebSocket mesh-server"""
    
    # Сигналы для передачи данных в UI
    metrics_received = Signal(dict)
    alert_received = Signal(dict)
    message_received = Signal(dict)
    device_updated = Signal(dict)
    connected = Signal()
    disconnected = Signal()
    error = Signal(str)
    
    def __init__(self, server_url: str = "http://localhost:8080"):
        super().__init__()
        self.server_url = server_url
        self.ws: Optional[WebSocketApp] = None
        self._running = False
        
    def connect(self, ws_url: Optional[str] = None):
        """Подключение к WebSocket"""
        if not ws_url:
            ws_url = self.server_url.replace("http", "ws")
            
        ws_url = ws_url.rstrip('/') + '/ws'
        
        self.ws = WebSocketApp(
            ws_url,
            on_open=self._on_open,
            on_message=self._on_message,
            on_error=self._on_error,
            on_close=self._on_close
        )
        
        self._running = True
        ws_thread = threading.Thread(target=self.ws.run_forever, daemon=True)
        ws_thread.start()
        
    def disconnect(self):
        """Отключение от WebSocket"""
        self._running = False
        if self.ws:
            self.ws.close()
            self.ws = None
            
    def _on_open(self, ws):
        """Обработчик подключения"""
        print("WebSocket connected")
        self.connected.emit()
        
    def _on_message(self, ws, message: str):
        """Обработчик входящих сообщений"""
        try:
            data = json.loads(message)
            msg_type = data.get('type', 'unknown')
            
            if msg_type == 'metrics':
                self.metrics_received.emit(data.get('data', {}))
            elif msg_type == 'alert':
                self.alert_received.emit(data.get('data', {}))
            elif msg_type == 'message':
                self.message_received.emit(data.get('data', {}))
            elif msg_type == 'device':
                self.device_updated.emit(data.get('data', {}))
            else:
                print(f"Unknown message type: {msg_type}")
                
        except json.JSONDecodeError as e:
            print(f"JSON decode error: {e}")
            
    def _on_error(self, ws, error):
        """Обработчик ошибок"""
        print(f"WebSocket error: {error}")
        self.error.emit(str(error))
        
    def _on_close(self, ws, close_status_code, close_msg):
        """Обработчик отключения"""
        print(f"WebSocket disconnected: {close_status_code} - {close_msg}")
        self.disconnected.emit()
        
        # Авто-реконнект если нужно
        if self._running:
            import time
            time.sleep(2)
            self.connect()
