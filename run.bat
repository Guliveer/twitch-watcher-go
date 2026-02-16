@echo off
setlocal

REM Build and run twitch-miner-go
REM Usage: scripts\run.bat [flags]
REM Example: scripts\run.bat -config configs -port 8080 -log-level debug

cd /d "%~dp0\.."

echo Building twitch-miner...
go build -o twitch-miner.exe ./cmd/miner
if %errorlevel% neq 0 (
    echo Build failed!
    exit /b %errorlevel%
)

echo Starting twitch-miner...
twitch-miner.exe %*
