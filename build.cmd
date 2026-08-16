@echo off
setlocal
cd /d "%~dp0"

set "WAILS=%USERPROFILE%\go\bin\wails.exe"
if not exist "%WAILS%" (
  echo Khong tim thay Wails CLI.
  echo Chay: go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
  exit /b 1
)

pushd "wails-app"
go test ./...
if errorlevel 1 (
  popd
  exit /b 1
)

"%WAILS%" build -clean -platform windows/amd64 -webview2 embed -nocolour
if errorlevel 1 (
  popd
  exit /b 1
)
popd

if not exist "dist" mkdir "dist"
copy /y "wails-app\build\bin\VideoHtmlDownloader.exe" "dist\VideoHtmlDownloader.exe" >nul
if errorlevel 1 exit /b 1

echo.
echo Da tao: dist\VideoHtmlDownloader.exe
exit /b 0
