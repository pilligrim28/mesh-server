# Сканирование BLE устройств для поиска Meshtastic/ESP32

Write-Host "=== Сканирование BLE устройств ===" -ForegroundColor Cyan
Write-Host "Поиск устройств Meshtastic/ESP32..." -ForegroundColor Yellow
Write-Host ""

$watcher = [Windows.Devices.Bluetooth.Advertisement.BluetoothLEAdvertisementWatcher]::new()
$devices = @()

$watcher.add_Received({
    param($sender, $args)
    $device = @{
        Address = $args.BluetoothAddress.ToString("X12")
        Name = $args.Advertisement.LocalName
        RSSI = $args.RawSignalStrengthInDBM
    }
    $devices += $device
    
    # Проверяем является ли устройство Meshtastic/ESP32
    $name = $args.Advertisement.LocalName
    if ($name -match "(?i)meshtastic|esp32|lilygo|heltec|tbeam|tracker") {
        Write-Host ">>> НАЙДЕНО УСТРОЙСТВО MESHTASTIC/ESP32 <<<" -ForegroundColor Green
        Write-Host "    Address: $($device.Address)" -ForegroundColor Green
        Write-Host "    Name: $($device.Name)" -ForegroundColor Green
        Write-Host "    RSSI: $($device.RSSI) dBm" -ForegroundColor Green
        Write-Host ""
    }
})

Write-Host "Сканирование... (5 секунд)" -ForegroundColor Gray
$watcher.Start()
Start-Sleep -Seconds 5
$watcher.Stop()

Write-Host ""
Write-Host "=== Все найденные BLE устройства ===" -ForegroundColor Cyan
if ($devices.Count -eq 0) {
    Write-Host "Устройства не найдены" -ForegroundColor Red
} else {
    foreach ($d in $devices) {
        $isMeshtastic = $d.Name -match "(?i)meshtastic|esp32|lilygo|heltec|tbeam|tracker"
        if ($isMeshtastic) {
            Write-Host "  [MESHTASTIC] " -NoNewline -ForegroundColor Green
        } else {
            Write-Host "  [DEVICE]     " -NoNewline -ForegroundColor Gray
        }
        Write-Host "Address: $($d.Address) | Name: '$($d.Name)' | RSSI: $($d.RSSI) dBm"
    }
}

Write-Host ""
Write-Host "Всего найдено устройств: $($devices.Count)" -ForegroundColor Cyan
Write-Host "Из них Meshtastic/ESP32: $(($devices | Where-Object { $_.Name -match '(?i)meshtastic|esp32|lilygo|heltec|tbeam|tracker' }).Count)" -ForegroundColor Cyan
