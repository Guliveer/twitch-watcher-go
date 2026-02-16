@echo off
setlocal

REM Build and run twitch-watcher-go
REM Usage: scripts\run.bat [flags]
REM Example: scripts\run.bat -config configs -port 8080 -log-level debug

cd /d "%~dp0\.."

echo Building twitch-watcher-go...
go build -o twitch-watcher-go.exe ./cmd/twitch-watcher-go
if %errorlevel% neq 0 (
    echo Build failed!
    exit /b %errorlevel%
)

echo Starting twitch-watcher-go...
twitch-watcher-go.exe %*
