@echo off
echo Building turing-display.exe...
go build -ldflags="-H windowsgui" -o turing-display.exe .
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)
echo Done.
