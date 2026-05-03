@echo off
setlocal

if not exist "%cd%\.dist" mkdir "%cd%\.dist"

echo Building Go project for Windows...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

go build -buildvcs=false -o ".dist\TsunamiDB.exe" .
if errorlevel 1 (
    echo Windows build failed!
    exit /b 1
)

echo Building Go project for Linux...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0

go build -buildvcs=false -o ".dist\TsunamiDB-linux" .
if errorlevel 1 (
    echo Linux build failed!
    exit /b 1
)

echo Setting icon for .dist\TsunamiDB.exe...
"%cd%\assets\rcedit-x64.exe" "%cd%\.dist\TsunamiDB.exe" --set-icon "%cd%\assets\Tsu.ico"
if errorlevel 1 (
    echo Failed to set icon!
    exit /b 1
)

echo Build and icon set successfully!
pause
