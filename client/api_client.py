"""
API клиент для взаимодействия с mesh-server
"""
import requests
from typing import List, Dict, Any, Optional


class MeshAPIClient:
    """Клиент для REST API mesh-server"""
    
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url.rstrip('/')
        self.session = requests.Session()
        self.session.headers.update({
            'Content-Type': 'application/json',
            'Accept': 'application/json'
        })
        
    def _request(self, method: str, endpoint: str, **kwargs) -> Optional[Dict]:
        """Выполнение HTTP запроса"""
        url = f"{self.base_url}{endpoint}"
        try:
            response = self.session.request(method, url, timeout=10, **kwargs)
            response.raise_for_status()
            
            if response.status_code == 204:
                return None
                
            return response.json()
        except requests.exceptions.RequestException as e:
            print(f"API Error: {e}")
            return None
            
    # ========== Devices ==========
    
    def get_devices(self) -> List[Dict]:
        """Получить все устройства"""
        result = self._request('GET', '/api/devices')
        return result if result else []
        
    def create_device(self, device_data: Dict) -> Optional[Dict]:
        """Создать устройство"""
        return self._request('POST', '/api/devices', json=device_data)
        
    # ========== Metrics ==========
    
    def get_metrics(self) -> List[Dict]:
        """Получить последние метрики всех устройств"""
        result = self._request('GET', '/api/metrics')
        return result if result else []
        
    def get_metrics_by_device(self, device_id: int) -> List[Dict]:
        """Получить метрики устройства"""
        return self._request('GET', f'/api/metrics/device?device_id={device_id}') or []
        
    def create_metric(self, metric_data: Dict) -> Optional[Dict]:
        """Создать метрику"""
        return self._request('POST', '/api/metrics', json=metric_data)
        
    # ========== Alerts ==========
    
    def get_alerts(self, unread_only: bool = True) -> List[Dict]:
        """Получить алерты"""
        endpoint = '/api/alerts' if unread_only else '/api/alerts/device'
        result = self._request('GET', endpoint)
        return result if result else []
        
    def get_alerts_by_device(self, device_id: int) -> List[Dict]:
        """Получить алерты устройства"""
        return self._request('GET', f'/api/alerts/device?device_id={device_id}') or []
        
    def create_alert(self, alert_data: Dict) -> Optional[Dict]:
        """Создать алерт"""
        return self._request('POST', '/api/alerts', json=alert_data)
        
    def mark_alert_read(self, alert_id: int) -> bool:
        """Пометить алерт как прочитанный"""
        result = self._request('PUT', f'/api/alerts/read?id={alert_id}')
        return result is not None
        
    def mark_all_alerts_read(self, device_id: Optional[int] = None) -> bool:
        """Пометить все алерты как прочитанные"""
        endpoint = f'/api/alerts/read-all'
        if device_id:
            endpoint += f'?device_id={device_id}'
        result = self._request('PUT', endpoint)
        return result is not None
        
    # ========== Messages ==========
    
    def get_messages(self) -> List[Dict]:
        """Получить исходящие сообщения"""
        result = self._request('GET', '/api/messages')
        return result if result else []
        
    def get_messages_by_device(self, device_id: int, limit: int = 50) -> List[Dict]:
        """Получить сообщения устройства"""
        return self._request('GET', f'/api/messages/device?device_id={device_id}&limit={limit}') or []
        
    def send_message(self, message_data: Dict) -> Optional[Dict]:
        """Отправить сообщение"""
        return self._request('POST', '/api/messages', json=message_data)
        
    # ========== Map ==========
    
    def get_map_data(self) -> List[Dict]:
        """Получить данные для карты"""
        result = self._request('GET', '/api/map')
        return result if result else []
        
    # ========== Discovery ==========
    
    def get_discovery_devices(self) -> List[Dict]:
        """Получить все обнаруженные устройства"""
        result = self._request('GET', '/api/discovery')
        return result if result else []
        
    def get_bluetooth_devices(self) -> List[Dict]:
        """Получить Bluetooth устройства"""
        result = self._request('GET', '/api/discovery/bluetooth')
        return result if result else []
        
    def get_wifi_devices(self) -> List[Dict]:
        """Получить WiFi устройства"""
        result = self._request('GET', '/api/discovery/wifi')
        return result if result else []
        
    def get_meshtastic_devices(self) -> List[Dict]:
        """Получить Meshtastic устройства"""
        result = self._request('GET', '/api/discovery/meshtastic')
        return result if result else []
        
    def get_discovery_status(self) -> Dict:
        """Получить статус сканирования"""
        return self._request('GET', '/api/discovery/status') or {}
        
    def start_scan(self, scan_type: str = 'all') -> bool:
        """Запустить сканирование"""
        result = self._request('POST', '/api/discovery/scan', json={'type': scan_type})
        return result is not None
        
    def start_bluetooth_scan(self) -> bool:
        """Запустить Bluetooth сканирование"""
        result = self._request('POST', '/api/discovery/bluetooth/start')
        return result is not None
        
    def stop_bluetooth_scan(self) -> bool:
        """Остановить Bluetooth сканирование"""
        result = self._request('POST', '/api/discovery/bluetooth/stop')
        return result is not None
        
    def start_wifi_scan(self) -> bool:
        """Запустить WiFi сканирование"""
        result = self._request('POST', '/api/discovery/wifi/start')
        return result is not None
        
    def stop_wifi_scan(self) -> bool:
        """Остановить WiFi сканирование"""
        result = self._request('POST', '/api/discovery/wifi/stop')
        return result is not None
        
    def clear_discovery(self) -> bool:
        """Очистить список обнаруженных устройств"""
        result = self._request('DELETE', '/api/discovery/clear')
        return result is not None
        
    def get_network_info(self) -> Dict:
        """Получить информацию о сетевых интерфейсах"""
        return self._request('GET', '/api/discovery/network') or {}
        
    # ========== Simulator ==========
    
    def get_simulator_status(self) -> Dict:
        """Получить статус симулятора"""
        return self._request('GET', '/api/simulator/status') or {}
        
    def start_simulator(self) -> bool:
        """Запустить симулятор"""
        result = self._request('POST', '/api/simulator/start')
        return result is not None
        
    def stop_simulator(self) -> bool:
        """Остановить симулятор"""
        result = self._request('POST', '/api/simulator/stop')
        return result is not None
        
    def trigger_simulator_event(self, event_type: str) -> bool:
        """Генерация события симулятора"""
        result = self._request('POST', '/api/simulator/event', json={'type': event_type})
        return result is not None
        
    # ========== Health ==========
    
    def health_check(self) -> bool:
        """Проверка работоспособности сервера"""
        try:
            response = self.session.get(f"{self.base_url}/health", timeout=5)
            return response.status_code == 200
        except:
            return False
