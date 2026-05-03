@echo off
setlocal

set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=1
set "SCRIPT_DIR=%~dp0"

if /I "%~1"=="linux" goto build_linux

if not exist "%SCRIPT_DIR%..\..\.dist" mkdir "%SCRIPT_DIR%..\..\.dist"

go build -buildvcs=false -buildmode=c-shared -o "%SCRIPT_DIR%..\..\.dist\tsunamidb.dll" "%SCRIPT_DIR%."
if errorlevel 1 (
    echo DLL build failed.
    exit /b 1
)

echo DLL build completed: .dist\tsunamidb.dll
exit /b 0

:build_linux
where wsl.exe >nul 2>nul
if errorlevel 1 (
    echo Linux shared library build requires Linux/WSL with a C compiler.
    echo Run: bash ./lib/dll/build.sh
    exit /b 1
)

set "WSL_DISTROS="
for /f "usebackq delims=" %%D in (`wsl.exe --list --quiet 2^>nul`) do set "WSL_DISTROS=%%D"
if not defined WSL_DISTROS (
    echo Linux shared library build requires a configured WSL distribution with a C compiler.
    echo Run on Linux/WSL: bash ./lib/dll/build.sh
    exit /b 1
)

for /f "usebackq delims=" %%I in (`wsl.exe wslpath -a "%SCRIPT_DIR%..\.."`) do set "WSL_REPO=%%I"
if not defined WSL_REPO (
    echo Failed to resolve repository path in WSL.
    exit /b 1
)

wsl.exe bash -lc "cd '%WSL_REPO%' && bash ./lib/dll/build.sh"
if errorlevel 1 (
    echo Linux shared library build failed.
    exit /b 1
)

echo Linux shared library build completed: .dist/libtsunamidb.so
