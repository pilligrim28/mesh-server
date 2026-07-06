#!/usr/bin/env python3
"""
Meshtastic BLE GATT Server для Raspberry Pi
Создаёт BLE GATT сервер с UUID сервисами Meshtastic,
чтобы приложение Meshtastic могло обнаружить и подключиться.
"""

import dbus
import dbus.service
import dbus.mainloop.glib
from gi.repository import GLib
import struct
import sys
import json
import os
import time
import threading

# Meshtastic Service UUID
SERVICE_UUID = "6ba1b218-15a8-461f-9fa8-511d75550f30"
# Characteristic UUIDs
TO_RADIO_UUID = "f8925b27-2b8f-4f60-94dc-9e0810c91b8f"
FROM_RADIO_UUID = "85b01054-68e8-45b0-8c7b-db15387dd36c"
FROM_NUM_UUID = "73b34405-de6d-4f40-b3b4-43e5c34b63d4"
TO_NUM_UUID = "c571e88c-2c2f-49bc-8c55-48efde4ea67e"

BLUEZ_SERVICE_NAME = "org.bluez"
GATT_MANAGER_IFACE = "org.bluez.GattManager1"
LE_ADVERTISING_MANAGER_IFACE = "org.bluez.LEAdvertisingManager1"
ADAPTER_IFACE = "org.bluez.Adapter1"
DBUS_OM_IFACE = "org.freedesktop.DBus.ObjectManager"
DBUS_PROP_IFACE = "org.freedesktop.DBus.Properties"

# HTTP API port for Meshtastic
HTTP_API_PORT = 4403


def uuid_to_short(uuid_str):
    """Convert full UUID to 16-bit short UUID"""
    return int(uuid_str.split("-")[0], 16)


class Characteristic(dbus.service.Object):
    """BLE Characteristic"""

    def __init__(self, uuid, flags, service):
        self.path = service.path + "/char" + str(uuid_to_short(uuid))
        self.uuid = uuid
        self.flags = flags
        self.service = service
        self.value = []
        self.notifying = False
        self.write_callback = None
        self.read_callback = None
        dbus.service.Object.__init__(self, service.bus, self.path)

    def get_properties(self):
        return {
            "org.bluez.GattCharacteristic1": {
                "Service": self.service.get_path(),
                "UUID": self.uuid,
                "Flags": self.flags,
            }
        }

    def get_path(self):
        return dbus.ObjectPath(self.path)

    @dbus.service.method(DBUS_PROP_IFACE, in_signature="ss", out_signature="v")
    def Get(self, interface, prop):
        return self.get_properties()[interface][prop]

    @dbus.service.method(DBUS_PROP_IFACE, in_signature="s", out_signature="a{sv}")
    def GetAll(self, interface):
        return self.get_properties().get(interface, {})

    @dbus.service.method("org.bluez.GattCharacteristic1", in_signature="a{sv}", out_signature="ay")
    def ReadValue(self, options):
        if self.read_callback:
            self.value = self.read_callback()
        return self.value

    @dbus.service.method("org.bluez.GattCharacteristic1", in_signature="aya{sv}")
    def WriteValue(self, value, options):
        self.value = value
        if self.write_callback:
            self.write_callback(bytes(value))

    @dbus.service.method("org.bluez.GattCharacteristic1")
    def StartNotify(self):
        self.notifying = True

    @dbus.service.method("org.bluez.GattCharacteristic1")
    def StopNotify(self):
        self.notifying = False

    @dbus.service.signal(DBUS_PROP_IFACE, signature="sa{sv}as")
    def PropertiesChanged(self, interface, changed, invalidated):
        pass


class Service(dbus.service.Object):
    """BLE Service"""

    def __init__(self, uuid, primary, bus, index):
        self.path = "/org/bluez/hci0/service" + str(index)
        self.uuid = uuid
        self.primary = primary
        self.characteristics = []
        self.bus = bus
        dbus.service.Object.__init__(self, bus, self.path)

    def get_path(self):
        return dbus.ObjectPath(self.path)

    def add_characteristic(self, uuid, flags):
        ch = Characteristic(uuid, flags, self)
        self.characteristics.append(ch)
        return ch

    def get_properties(self):
        return {
            "org.bluez.GattService1": {
                "UUID": self.uuid,
                "Primary": self.primary,
                "Characteristics": dbus.Array(
                    [ch.get_path() for ch in self.characteristics],
                    signature="o"
                ),
            }
        }

    def get_path(self):
        return dbus.ObjectPath(self.path)

    @dbus.service.method(DBUS_PROP_IFACE, in_signature="ss", out_signature="v")
    def Get(self, interface, prop):
        return self.get_properties()[interface][prop]

    @dbus.service.method(DBUS_PROP_IFACE, in_signature="s", out_signature="a{sv}")
    def GetAll(self, interface):
        return self.get_properties().get(interface, {})


class Advertisement(dbus.service.Object):
    """BLE Advertisement"""

    def __init__(self, index, local_name, service_uuids):
        self.path = "/org/bluez/hci0/adv" + str(index)
        self.local_name = local_name
        self.service_uuids = service_uuids
        dbus.service.Object.__init__(self, dbus.SessionBus(), self.path)

    def get_properties(self):
        return {
            "org.bluez.LEAdvertisement1": {
                "Type": "peripheral",
                "LocalName": self.local_name,
                "ServiceUUIDs": dbus.Array(self.service_uuids, signature="s"),
                "Includes": dbus.Array(["tx-power"], signature="s"),
            }
        }

    def get_path(self):
        return dbus.ObjectPath(self.path)

    @dbus.service.method(DBUS_PROP_IFACE, in_signature="ss", out_signature="v")
    def Get(self, interface, prop):
        return self.get_properties()[interface][prop]

    @dbus.service.method(DBUS_PROP_IFACE, in_signature="s", out_signature="a{sv}")
    def GetAll(self, interface):
        return self.get_properties().get(interface, {})

    @dbus.service.method("org.bluez.LEAdvertisement1")
    def Release(self):
        print("Advertisement released")


class Application(dbus.service.Object):
    """GATT Application"""

    def __init__(self, bus, index):
        self.path = "/org/bluez/hci0/gatt" + str(index)
        self.services = []
        dbus.service.Object.__init__(self, bus, self.path)

    def get_path(self):
        return dbus.ObjectPath(self.path)

    def add_service(self, service):
        self.services.append(service)

    @dbus.service.method(DBUS_OM_IFACE, out_signature="a{oa{sa{sv}}}")
    def GetManagedObjects(self):
        objects = {}
        for service in self.services:
            objects[service.get_path()] = service.get_properties()
            for char in service.characteristics:
                objects[char.get_path()] = char.get_properties()
        return objects


class MeshtasticBLEServer:
    """Meshtastic BLE GATT Server"""

    def __init__(self, device_name="Meshtastic Hub"):
        self.device_name = device_name
        self.app = None
        self.advertisement = None
        self.bus = None
        self.mainloop = None
        self.running = False
        self.from_radio_queue = []
        self.to_radio_data = None

    def start(self):
        """Start BLE GATT server"""
        try:
            dbus.mainloop.glib.DBusGMainLoop(set_as_default=True)
            self.bus = dbus.SystemBus()
            self.mainloop = GLib.MainLoop()

            # Find the adapter
            adapter_path = self._find_adapter()
            if not adapter_path:
                print("ERROR: No Bluetooth adapter found")
                return False

            # Create GATT application
            self.app = Application(self.bus, 0)
            self._create_services()

            # Register GATT application
            self._register_gatt(adapter_path)

            # Create and register advertisement
            self._register_advertisement(adapter_path)

            # Set powered on
            self._set_powered(adapter_path, True)

            self.running = True
            print(f"BLE Server started: {self.device_name}")
            print(f"Service UUID: {SERVICE_UUID}")
            print(f"Advertising: ON")

            # Run mainloop in a thread
            self.mainloop_thread = threading.Thread(target=self.mainloop.run, daemon=True)
            self.mainloop_thread.start()

            return True

        except Exception as e:
            print(f"ERROR starting BLE server: {e}")
            import traceback
            traceback.print_exc()
            return False

    def stop(self):
        """Stop BLE GATT server"""
        self.running = False
        if self.mainloop:
            self.mainloop.quit()
        print("BLE Server stopped")

    def _find_adapter(self):
        """Find the first Bluetooth adapter"""
        try:
            manager = dbus.Interface(
                self.bus.get_object(BLUEZ_SERVICE_NAME, "/"),
                DBUS_OM_IFACE
            )
            objects = manager.GetManagedObjects()
            for path, interfaces in objects.items():
                if ADAPTER_IFACE in interfaces:
                    return path
        except Exception as e:
            print(f"Error finding adapter: {e}")
        return None

    def _create_services(self):
        """Create Meshtastic GATT services and characteristics"""
        # Meshtastic Service
        service = Service(SERVICE_UUID, True, self.bus, 0)

        # From Radio (read + notify) - server sends data to phone
        from_radio = service.add_characteristic(
            FROM_RADIO_UUID,
            ["read", "notify"]
        )
        from_radio.read_callback = self._read_from_radio

        # To Radio (write) - phone sends data to server
        to_radio = service.add_characteristic(
            TO_RADIO_UUID,
            ["write", "write-without-response"]
        )
        to_radio.write_callback = self._write_to_radio

        # From Num (read) - number of packets available
        from_num = service.add_characteristic(
            FROM_NUM_UUID,
            ["read"]
        )
        from_num.read_callback = self._read_from_num

        # To Num (read) - number of packets sent
        to_num = service.add_characteristic(
            TO_NUM_UUID,
            ["read"]
        )
        to_num.read_callback = self._read_to_num

        self.app.add_service(service)
        self.from_radio_char = from_radio

    def _read_from_radio(self):
        """Read from FromRadio characteristic"""
        if self.from_radio_queue:
            return list(self.from_radio_queue.pop(0))
        return [0]

    def _write_to_radio(self, data):
        """Write to ToRadio characteristic"""
        self.to_radio_data = bytes(data)
        print(f"ToRadio received: {len(data)} bytes")

    def _read_from_num(self):
        """Read FromNum - number of available packets"""
        return [len(self.from_radio_queue) & 0xFF]

    def _read_to_num(self):
        """Read ToNum - always 0"""
        return [0]

    def _register_gatt(self, adapter_path):
        """Register GATT application with BlueZ"""
        try:
            gatt_manager = dbus.Interface(
                self.bus.get_object(BLUEZ_SERVICE_NAME, adapter_path),
                GATT_MANAGER_IFACE
            )
            gatt_manager.RegisterApplication(
                self.app.get_path(),
                {},
                reply_handler=self._register_gatt_reply,
                error_handler=self._register_gatt_error
            )
        except Exception as e:
            print(f"Error registering GATT: {e}")

    def _register_gatt_reply(self):
        print("GATT application registered")

    def _register_gatt_error(self, error):
        print(f"GATT registration error: {error}")

    def _register_advertisement(self, adapter_path):
        """Register BLE advertisement"""
        try:
            self.advertisement = Advertisement(
                0, self.device_name, [SERVICE_UUID]
            )

            ad_manager = dbus.Interface(
                self.bus.get_object(BLUEZ_SERVICE_NAME, adapter_path),
                LE_ADVERTISING_MANAGER_IFACE
            )
            ad_manager.RegisterAdvertisement(
                self.advertisement.get_path(),
                {},
                reply_handler=self._register_adv_reply,
                error_handler=self._register_adv_error
            )
        except Exception as e:
            print(f"Error registering advertisement: {e}")

    def _register_adv_reply(self):
        print("Advertisement registered")

    def _register_adv_error(self, error):
        print(f"Advertisement registration error: {error}")

    def _set_powered(self, adapter_path, powered):
        """Set adapter powered state"""
        try:
            props = dbus.Interface(
                self.bus.get_object(BLUEZ_SERVICE_NAME, adapter_path),
                DBUS_PROP_IFACE
            )
            props.Set(ADAPTER_IFACE, "Powered", dbus.Boolean(powered))
            props.Set(ADAPTER_IFACE, "Alias", self.device_name)
            print(f"Adapter powered: {powered}, alias: {self.device_name}")
        except Exception as e:
            print(f"Error setting powered: {e}")

    def send_to_phone(self, data):
        """Queue data to send to phone via FromRadio"""
        self.from_radio_queue.append(data)
        if self.from_radio_char and self.from_radio_char.notifying:
            self.from_radio_char.PropertiesChanged(
                "org.bluez.GattCharacteristic1",
                {"Value": dbus.Array(list(data), signature="y")},
                []
            )

    def get_to_radio(self):
        """Get data written by phone via ToRadio"""
        data = self.to_radio_data
        self.to_radio_data = None
        return data


def main():
    """Main entry point"""
    device_name = sys.argv[1] if len(sys.argv) > 1 else "Meshtastic Hub"
    port = int(sys.argv[2]) if len(sys.argv) > 2 else HTTP_API_PORT

    server = MeshtasticBLEServer(device_name)

    if not server.start():
        print("Failed to start BLE server")
        sys.exit(1)

    print(f"\nMeshtastic BLE Server running")
    print(f"  Device: {device_name}")
    print(f"  HTTP API: http://0.0.0.0:{port}")
    print(f"  BLE Service: {SERVICE_UUID}")
    print(f"\nPress Ctrl+C to stop\n")

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        server.stop()


if __name__ == "__main__":
    main()
