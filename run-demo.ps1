# Demo mode launcher for mesh-server
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not (Test-Path ".env")) {
    Copy-Item ".env.demo" ".env"
    Write-Host ".env created from .env.demo" -ForegroundColor Green
}

$env:DEMO_MODE = "true"

Write-Host ""
Write-Host "=== Mesh Server DEMO ===" -ForegroundColor Cyan
Write-Host "Browser: http://localhost:8080" -ForegroundColor Yellow
Write-Host "ESP32 USB hub: set ESP32_COM_PORT in .env (e.g. COM4)" -ForegroundColor Gray
Write-Host ""

go run .
