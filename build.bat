@echo off
setlocal
cd /d "%~dp0"

rem Use a China-accessible module proxy unless the user overrides it.
if "%GOPROXY%"=="" set GOPROXY=https://goproxy.cn,direct

rem Regenerate the Windows resource (manifest + icon) if rsrc is available.
where rsrc >nul 2>nul
if errorlevel 1 (
  echo [!] rsrc not found - skipping resource generation.
  echo     Install it with:  go install github.com/akavel/rsrc@latest
) else (
  echo [*] rsrc -manifest app.manifest -ico icon.ico -o rsrc.syso
  rsrc -manifest app.manifest -ico icon.ico -o rsrc.syso || exit /b 1
)

if not exist build mkdir build
echo [*] go build
go build -ldflags "-H=windowsgui -s -w" -o build\VerPinG.exe . || exit /b 1
copy /y icon.ico build\icon.ico >nul

echo [+] Built build\VerPinG.exe
endlocal
