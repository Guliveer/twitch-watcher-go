@echo off
setlocal

REM Build and run twitch-miner-go
REM Usage: scripts\run.bat [flags]
REM Example: scripts\run.bat -config configs -port 8080 -log-level debug

cd /d "%~dp0\.."

echo Building twitch-miner-go...
go build -o twitch-miner-go.exe ./cmd/twitch-miner-go
if %errorlevel% neq 0 (
    echo Build failed!
    exit /b %errorlevel%
)

echo Starting twitch-miner-go...
twitch-miner-go.exe %*
