@echo off
rem Запуск сервера на Windows. Bluetooth должен быть включён в параметрах Windows.
cd /d "%~dp0"
if not exist .env copy .env.example .env
start "" http://localhost:8080
mesh-server.exe
pause
