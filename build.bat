@echo off
REM Budowanie projektu Go na Windows
echo Building Go project for Windows...
set GOOS=windows
set GOARCH=amd64

go build -o TsunamiDB.exe

REM Sprawdź, czy build zakończył się sukcesem
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)

REM Budowanie projektu Go na Linuxa
echo Building Go project for Linux...
set GOOS=linux
set GOARCH=amd64

go build -o TsunamiDB-linux

REM Sprawdź, czy build zakończył się sukcesem
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)

echo Linux build completed successfully!

REM Ustawienie ikony dla pliku wykonywalnego Windows
echo Setting icon for TsunamiDB.exe...
"%cd%\assets\rcedit-x64.exe" "%cd%\TsunamiDB.exe" --set-icon "%cd%\assets\Tsus.ico"

REM Sprawdź, czy rcedit-x64.exe zakończył się sukcesem
if errorlevel 1 (
    echo Failed to set icon!
    exit /b 1
)

echo Build and icon set successfully!
pause
