@echo off
setlocal

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=1
set "SCRIPT_DIR=%~dp0"

if not exist "%SCRIPT_DIR%..\..\.dist" mkdir "%SCRIPT_DIR%..\..\.dist"

go build -buildmode=c-shared -o "%SCRIPT_DIR%..\..\.dist\tsunamidb.dll" "%SCRIPT_DIR%."
if errorlevel 1 (
    echo DLL build failed.
    exit /b 1
)

echo DLL build completed: .dist\tsunamidb.dll
