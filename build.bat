@echo off
echo Formatting Go files...
gofmt -w .
if errorlevel 1 (
    echo Format failed!
    exit /b 1
)
echo Linting turing-display...
go vet ./...
if errorlevel 1 (
    echo Lint failed!
    exit /b 1
)
echo Testing turing-display...
go test ./...
if errorlevel 1 (
    echo Test failed!
    exit /b 1
)
if not exist dist mkdir dist
echo Building Windows resources...
go run github.com/tc-hib/go-winres@latest make --in build\windows\winres.json --out cmd\rsrc
if errorlevel 1 (
    echo Resource build failed!
    exit /b 1
)
echo Building turing-display.exe...
go build -ldflags="-H windowsgui" -o dist\turing-display.exe ./cmd
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)
copy /Y config\config.json dist\config.json >nul
if errorlevel 1 (
    echo Config copy failed!
    exit /b 1
)
echo Done.
