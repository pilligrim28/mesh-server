# Скрипт для исправления проблемы с кириллицей в пути Arduino
# Запускать от имени администратора!

Write-Host "=== Исправление пути Arduino для совместимости с toolchain ===" -ForegroundColor Cyan

# Пути - создаём симлинк для всей папки Users\Александр
$sourcePath = "C:\Users\Александр"
$targetLink = "C:\Users\Alex"

# Проверка прав администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Host "ERROR: Запустите скрипт от имени администратора!" -ForegroundColor Red
    Write-Host "Правый клик на PowerShell -> Запустить от имени администратора" -ForegroundColor Yellow
    exit 1
}

Write-Host "`n[1/5] Проверка исходного пути..." -ForegroundColor Green

if (-not (Test-Path $sourcePath)) {
    Write-Host "ERROR: Исходный путь не найден: $sourcePath" -ForegroundColor Red
    exit 1
}

Write-Host "Исходный путь найден: $sourcePath" -ForegroundColor Gray

Write-Host "`n[2/5] Проверка целевого пути..." -ForegroundColor Green

if (Test-Path $targetLink) {
    Write-Host "WARNING: Целевой путь уже существует: $targetLink" -ForegroundColor Yellow
    $response = Read-Host "Удалить и продолжить? (y/n)"
    if ($response -eq 'y') {
        Remove-Item $targetLink -Recurse -Force
        Write-Host "Удалено: $targetLink" -ForegroundColor Gray
    } else {
        Write-Host "Отмена операции" -ForegroundColor Yellow
        exit 0
    }
}

Write-Host "`n[3/5] Создание симлинка..." -ForegroundColor Green

# Создание directory symlink
$result = cmd.exe /C "mklink /D `"$targetLink`" `"$sourcePath`" 2>&1"
Write-Host $result -ForegroundColor Gray

if ($result -like "*created*") {
    Write-Host "SUCCESS: Симлинк создан" -ForegroundColor Green
    Write-Host "  $targetLink -> $sourcePath" -ForegroundColor Gray
} else {
    Write-Host "ERROR: Не удалось создать симлинк" -ForegroundColor Red
    Write-Host "Попробуйте вручную: mklink /D $targetLink $sourcePath" -ForegroundColor Yellow
    exit 1
}

Write-Host "`n[4/5] Создание C:\Alexander для короткого пути..." -ForegroundColor Green

$shortLink = "C:\Alexander"
if (Test-Path $shortLink) {
    Remove-Item $shortLink -Force
}
cmd.exe /C "mklink /D `"$shortLink`" `"$sourcePath`""
Write-Host "Создан: $shortLink -> $sourcePath" -ForegroundColor Gray

Write-Host "`n[5/5] Настройка переменных окружения..." -ForegroundColor Green

# Установка переменной окружения для Arduino
[Environment]::SetEnvironmentVariable("ARDUINO_DATA_DIR", "$targetLink\AppData\Local\arduino", "User")
Write-Host "Установлена переменная ARDUINO_DATA_DIR" -ForegroundColor Gray

Write-Host "`n=== Готово! ===" -ForegroundColor Cyan
Write-Host "`nСледующие шаги:" -ForegroundColor White
Write-Host "1. Закройте Arduino IDE (если открыто)" -ForegroundColor Gray
Write-Host "2. Откройте Arduino IDE" -ForegroundColor Gray
Write-Host "3. File -> Preferences -> Sketchbook location: $targetLink\AppData\Local\arduino" -ForegroundColor Gray
Write-Host "4. Попробуйте скомпилировать снова" -ForegroundColor Gray

Write-Host "`nАльтернативный путь для компиляции:" -ForegroundColor Yellow
Write-Host "  Используйте путь: C:\Alexander\AppData\Local\arduino" -ForegroundColor Gray
