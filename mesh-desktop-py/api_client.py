"""
API клиент для подключения к mesh-server
"""
import requests
from typing import List, Dict, Optional
from dataclasses import dataclass
from datetime import datetime


@dataclass
class Device:
    id: int
    node_id: str
    name: str
    latitude: float
    longitude: float
    altitude: float
    last_seen: str
    is_online: bool = False


@dataclass
class Metric:
    id: int
    device_id: int
    heart_rate: int
    co2: int
    temp: float
    humidity: float
    timestamp: str


@dataclass
class Alert:
    id: int
    device_id: int
    type: str
    message: str
    severity: str
    read: bool
    timestamp: str


@dataclass
class Message:
    id: int
    device_id: int
    from_node: str
    to_node: str
    text: str
    direction: str
    timestamp: str


class MeshServerClient:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.timeout = 10

    def check_connection(self) -> bool:
        """Проверка подключения к серверу"""
        try:
            resp = self.session.get(f"{self.base_url}/health")
            return resp.status_code == 200
        except Exception:
            return False

    def get_devices(self) -> List[Device]:
        """Получить все устройства"""
        try:
            resp = self.session.get(f"{self.base_url}/api/devices")
            if resp.status_code != 200:
                return []
            data = resp.json()
            devices = []
            for d in data:
                device = Device(
                    id=d.get("id", 0),
                    node_id=d.get("node_id", ""),
                    name=d.get("name", ""),
                    latitude=d.get("latitude", 0),
                    longitude=d.get("longitude", 0),
                    altitude=d.get("altitude", 0),
                    last_seen=d.get("last_seen", ""),
                )
                # Проверяем онлайн статус
                try:
                    last_seen = datetime.fromisoformat(device.last_seen.replace("Z", "+00:00"))
                    device.is_online = (datetime.now(last_seen.tzinfo) - last_seen).total_seconds() < 300
                except Exception:
                    device.is_online = False
                devices.append(device)
            return devices
        except Exception:
            return []

    def get_metrics(self) -> List[Metric]:
        """Получить последние метрики"""
        try:
            resp = self.session.get(f"{self.base_url}/api/metrics")
            if resp.status_code != 200:
                return []
            data = resp.json()
            return [
                Metric(
                    id=m.get("id", 0),
                    device_id=m.get("device_id", 0),
                    heart_rate=m.get("heart_rate", 0),
                    co2=m.get("co2", 0),
                    temp=m.get("temp", 0),
                    humidity=m.get("humidity", 0),
                    timestamp=m.get("timestamp", ""),
                )
                for m in data
            ]
        except Exception:
            return []

    def get_unread_alerts(self) -> List[Alert]:
        """Получить непрочитанные алерты"""
        try:
            resp = self.session.get(f"{self.base_url}/api/alerts")
            if resp.status_code != 200:
                return []
            data = resp.json()
            return [
                Alert(
                    id=a.get("id", 0),
                    device_id=a.get("device_id", 0),
                    type=a.get("type", ""),
                    message=a.get("message", ""),
                    severity=a.get("severity", ""),
                    read=a.get("read", False),
                    timestamp=a.get("timestamp", ""),
                )
                for a in data
            ]
        except Exception:
            return []

    def mark_alert_read(self, alert_id: int) -> bool:
        """Пометить алерт как прочитанный"""
        try:
            resp = self.session.put(f"{self.base_url}/api/alerts/read?id={alert_id}")
            return resp.status_code == 200
        except Exception:
            return False

    def mark_all_alerts_read(self, device_id: int = 0) -> bool:
        """Пометить все алерты как прочитанные"""
        try:
            url = f"{self.base_url}/api/alerts/read-all"
            if device_id > 0:
                url += f"?device_id={device_id}"
            resp = self.session.put(url)
            return resp.status_code == 200
        except Exception:
            return False

    def get_messages(self, device_id: int, limit: int = 50) -> List[Message]:
        """Получить сообщения устройства"""
        try:
            resp = self.session.get(
                f"{self.base_url}/api/messages/device?device_id={device_id}&limit={limit}"
            )
            if resp.status_code != 200:
                return []
            data = resp.json()
            return [
                Message(
                    id=m.get("id", 0),
                    device_id=m.get("device_id", 0),
                    from_node=m.get("from_node", ""),
                    to_node=m.get("to_node", ""),
                    text=m.get("text", ""),
                    direction=m.get("direction", ""),
                    timestamp=m.get("timestamp", ""),
                )
                for m in data
            ]
        except Exception:
            return []

    def send_message(
        self, device_id: int, from_node: str, to_node: str, text: str
    ) -> bool:
        """Отправить сообщение"""
        try:
            payload = {
                "device_id": device_id,
                "from_node": from_node,
                "to_node": to_node,
                "text": text,
                "direction": "outbound",
            }
            resp = self.session.post(
                f"{self.base_url}/api/messages", json=payload
            )
            return resp.status_code == 200
        except Exception:
            return False

    def get_discovered_devices(self) -> List[Dict]:
        """Получить обнаруженные устройства"""
        try:
            resp = self.session.get(f"{self.base_url}/api/discovery")
            if resp.status_code != 200:
                return []
            return resp.json()
        except Exception:
            return []

    def start_bluetooth_scan(self) -> bool:
        """Запустить Bluetooth сканирование"""
        try:
            resp = self.session.post(
                f"{self.base_url}/api/discovery/bluetooth/start"
            )
            return resp.status_code == 200
        except Exception:
            return False

    def stop_bluetooth_scan(self) -> bool:
        """Остановить Bluetooth сканирование"""
        try:
            resp = self.session.post(
                f"{self.base_url}/api/discovery/bluetooth/stop"
            )
            return resp.status_code == 200
        except Exception:
            return False

    def start_wifi_scan(self) -> bool:
        """Запустить WiFi сканирование"""
        try:
            resp = self.session.post(f"{self.base_url}/api/discovery/wifi/start")
            return resp.status_code == 200
        except Exception:
            return False

    def stop_wifi_scan(self) -> bool:
        """Остановить WiFi сканирование"""
        try:
            resp = self.session.post(f"{self.base_url}/api/discovery/wifi/stop")
            return resp.status_code == 200
        except Exception:
            return False

    def get_discovery_status(self) -> Dict:
        """Получить статус сканирования"""
        try:
            resp = self.session.get(f"{self.base_url}/api/discovery/status")
            if resp.status_code != 200:
                return {
                    "bluetooth_scanning": False,
                    "wifi_scanning": False,
                    "devices_found": 0,
                }
            return resp.json()
        except Exception:
            return {
                "bluetooth_scanning": False,
                "wifi_scanning": False,
                "devices_found": 0,
            }
