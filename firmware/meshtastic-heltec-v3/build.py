#!/usr/bin/env python3
"""
Сборка прошивок Meshtastic для Heltec WiFi LoRa 32 V3 с настройками сети Mesh Pulse.

  gateway — шлюз у сервера: Bluetooth без PIN, сервер подключается к нему сам.
  node    — носимый узел: Bluetooth с фиксированным PIN, к нему подключается ESP32-C3 с датчиком.

Все узлы получают одинаковые регион, канал и ключ шифрования (PSK), поэтому сразу видят друг друга.
Ключ генерируется при первом запуске и сохраняется в settings.json — храните его, без него новые
узлы не попадут в вашу сеть.

  python build.py              # собрать обе прошивки
  python build.py gateway      # только шлюз
  python build.py --fw-dir ../../../mfw node   # использовать уже скачанные исходники

Требуется: git, Python 3.9+, PlatformIO (pip install platformio).
"""
import argparse, base64, json, re, secrets, shutil, subprocess, sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
SETTINGS = HERE / "settings.json"
DEFAULTS = {
    "firmware_repo": "https://github.com/meshtastic/firmware",
    "firmware_tag": "v2.7.26.54e0d8d",
    "region": "RU",                 # RU, EU_868, EU_433, US, ... (meshtastic_Config_LoRaConfig_RegionCode_*)
    "modem_preset": "LONG_FAST",    # LONG_FAST, MEDIUM_FAST, SHORT_FAST, LONG_SLOW ...
    "channel_name": "MeshPulse",
    "psk_base64": "",               # пусто = сгенерировать 256-битный ключ
    "node_ble_pin": 123456,         # PIN носимых узлов = MESH_BLE_PIN в прошивке ESP32-C3
    "battery_report_interval_s": 300,
}
NO_PIN_PATCH_MARK = "USERPREFS_BLUETOOTH_NO_PIN"


def load_settings():
    s = dict(DEFAULTS)
    if SETTINGS.exists():
        s.update(json.loads(SETTINGS.read_text(encoding="utf-8")))
    if not s["psk_base64"]:
        s["psk_base64"] = base64.b64encode(secrets.token_bytes(32)).decode()
        print(f"Сгенерирован новый ключ канала, сохранён в {SETTINGS.name}")
    SETTINGS.write_text(json.dumps(s, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    psk = base64.b64decode(s["psk_base64"])
    if len(psk) not in (16, 32):
        sys.exit("psk_base64 должен содержать 16 или 32 байта")
    if not (100000 <= int(s["node_ble_pin"]) <= 999999):
        sys.exit("node_ble_pin — шесть цифр")
    return s, psk


def user_prefs(s, psk, profile):
    p = {
        "USERPREFS_CONFIG_LORA_REGION": f"meshtastic_Config_LoRaConfig_RegionCode_{s['region']}",
        "USERPREFS_LORACONFIG_MODEM_PRESET": f"meshtastic_Config_LoRaConfig_ModemPreset_{s['modem_preset']}",
        "USERPREFS_CHANNELS_TO_WRITE": "1",
        "USERPREFS_CHANNEL_0_NAME": s["channel_name"],
        "USERPREFS_CHANNEL_0_PSK": "{ " + ", ".join(f"0x{b:02x}" for b in psk) + " }",
        "USERPREFS_CHANNEL_0_UPLINK_ENABLED": "false",
        "USERPREFS_CHANNEL_0_DOWNLINK_ENABLED": "false",
        "USERPREFS_CONFIG_LORA_IGNORE_MQTT": "true",
        "USERPREFS_CONFIG_DEVICE_ROLE": "meshtastic_Config_DeviceConfig_Role_CLIENT",
        "USERPREFS_CONFIG_DEVICE_TELEM_UPDATE_INTERVAL": str(s["battery_report_interval_s"]),
        "USERPREFS_CONFIG_SMART_POSITION_ENABLED": "false",
        "USERPREFS_CONFIG_POSITION_BROADCAST_INTERVAL": "3600",
    }
    if profile == "gateway":
        p["USERPREFS_BLUETOOTH_NO_PIN"] = "1"   # см. patch_no_pin()
        p["USERPREFS_CONFIG_OWNER_LONG_NAME"] = "MeshPulse Gateway"
        p["USERPREFS_CONFIG_OWNER_SHORT_NAME"] = "GW"
    else:
        p["USERPREFS_FIXED_BLUETOOTH"] = str(int(s["node_ble_pin"]))
    return p


def patch_no_pin(fw: Path):
    """Добавляет в прошивку опцию USERPREFS_BLUETOOTH_NO_PIN (в оригинале её нет):
    шлюз принимает BLE-подключение без сопряжения — так сервер на Windows и Linux
    подключается автоматически, без ввода PIN в системе."""
    f = fw / "src/mesh/NodeDB.cpp"
    src = f.read_text(encoding="utf-8")
    if NO_PIN_PATCH_MARK in src:
        return
    anchor = ": meshtastic_Config_BluetoothConfig_PairingMode_FIXED_PIN;\n#endif\n"
    if anchor not in src:
        sys.exit("Не удалось применить патч NO_PIN: изменилась структура NodeDB.cpp в этой версии прошивки")
    src = src.replace(anchor, anchor +
        "#ifdef USERPREFS_BLUETOOTH_NO_PIN // Mesh Pulse: шлюз без PIN\n"
        "    config.bluetooth.mode = meshtastic_Config_BluetoothConfig_PairingMode_NO_PIN;\n"
        "#endif\n", 1)
    f.write_text(src, encoding="utf-8")
    print("Патч NO_PIN применён к NodeDB.cpp")


def ensure_firmware(s, fw: Path):
    if not fw.exists():
        print(f"Скачиваю прошивку Meshtastic {s['firmware_tag']} …")
        subprocess.check_call(["git", "clone", "--depth", "1", "--branch", s["firmware_tag"], "--recurse-submodules",
                               "--shallow-submodules", s["firmware_repo"], str(fw)])
    return fw


def pio():
    for c in ("pio", "platformio"):
        if shutil.which(c):
            return [c]
    return [sys.executable, "-m", "platformio"]


def build(s, psk, fw: Path, profile: str, out: Path, base: dict):
    # исходный userPrefs.jsonc содержит обязательные ключи (мелодия, часовой пояс) — дополняем его
    prefs = {**base, **user_prefs(s, psk, profile)}
    (fw / "userPrefs.jsonc").write_text(json.dumps(prefs, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"\n=== Сборка {profile} (Heltec V3, {s['region']}, канал «{s['channel_name']}») ===")
    subprocess.check_call(pio() + ["run", "-e", "heltec-v3"], cwd=fw)
    build_dir = fw / ".pio/build/heltec-v3"
    dst = out / profile
    dst.mkdir(parents=True, exist_ok=True)
    copied = []
    for b in build_dir.glob("firmware-heltec-v3-*.bin"):
        kind = "factory" if b.name.endswith(".factory.bin") else "update"
        name = f"meshpulse-{profile}-heltec-v3-{s['firmware_tag']}.{kind}.bin"
        shutil.copy2(b, dst / name)
        copied.append(name)
    print("Готово:", ", ".join(sorted(copied)), "→", dst)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("profiles", nargs="*", help="gateway и/или node (по умолчанию обе)")
    ap.add_argument("--fw-dir", default=str(HERE / ".meshtastic-firmware"), help="каталог с исходниками Meshtastic")
    ap.add_argument("--out", default=str(HERE / "out"))
    a = ap.parse_args()
    profiles = a.profiles or ["gateway", "node"]
    if bad := [p for p in profiles if p not in ("gateway", "node")]:
        ap.error(f"неизвестный профиль: {bad}")
    s, psk = load_settings()
    fw = ensure_firmware(s, Path(a.fw_dir).resolve())
    patch_no_pin(fw)
    original = (fw / "userPrefs.jsonc").read_text(encoding="utf-8")
    base = json.loads(re.sub("//.*", "", original))
    try:
        for p in profiles:
            build(s, psk, fw, p, Path(a.out), base)
    finally:
        (fw / "userPrefs.jsonc").write_text(original, encoding="utf-8")
    print("\nПрошивка: см. README.md → «Прошивка Heltec V3». Используйте *.factory.bin с полной очисткой памяти.")


if __name__ == "__main__":
    main()
