@echo off
REM Фиксируем версию Wails CLI, соответствующую go.mod
set WAILS_VERSION=v2.12.0

echo Installing Wails CLI %WAILS_VERSION%...
go install github.com/wailsapp/wails/v2/cmd/wails@%WAILS_VERSION%

echo Building project...
wails build

echo Done.