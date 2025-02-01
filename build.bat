@echo off
chcp 65001 >nul

echo 生成版本信息...
goversioninfo -platform-specific=true
if errorlevel 1 (
    echo 生成版本信息失败
    exit /b 1
)

echo 正在编译...
del /f /q resource.syso >nul 2>&1
goversioninfo -platform-specific=true
go build -ldflags="-s -w -X main.appKey=test-1 -X main.appSecret=E7x!kPq3$vL8Z#bN2^mYd5&sR9*KjW6" -o 7zrpw.exe
if errorlevel 1 (
    echo 编译失败
    exit /b 1
)

echo 正在使用 UPX 压缩...
upx -9 --best 7zrpw.exe
if errorlevel 1 (
    echo UPX 压缩失败
    exit /b 1
)

echo 完成！ 