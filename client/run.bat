@echo off
REM Скрипт запуска Mesh Server Client

cd /d "%~dp0"

REM Проверка виртуального окружения
if not exist "venv\Scripts\python.exe" (
    echo Создание виртуального окружения...
    python -m venv venv
)

REM Активация
call venv\Scripts\activate

REM Проверка зависимостей
echo Проверка зависимостей...
pip show PySide6 >nul 2>&1
if errorlevel 1 (
    echo Установка зависимостей...
    pip install PySide6 requests websocket-client
)

REM Запуск приложения
echo Запуск Mesh Server Client...
python main.py %*
