@echo off
setlocal

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=1

if not exist "%~dp0..\..\.dist" mkdir "%~dp0..\..\.dist"

go build -buildmode=c-shared -o "%~dp0..\..\.dist\tsunamidb.dll" "%~dp0"
if errorlevel 1 (
    echo DLL build failed.
    exit /b 1
)

echo DLL build completed: .dist\tsunamidb.dll
