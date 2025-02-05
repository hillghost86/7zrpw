@echo off
chcp 65001 >nul

:: 清理旧的资源文件
echo 清理旧的资源文件...
del /f /q resource_windows_386.syso >nul 2>&1
del /f /q resource_windows_amd64.syso >nul 2>&1
del /f /q resource_windows_arm.syso >nul 2>&1
del /f /q resource_windows_arm64.syso >nul 2>&1


:: 生成新的版本信息
echo 生成版本信息...
goversioninfo -platform-specific=true
if errorlevel 1 (
    echo 版本信息生成失败
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

echo 编译完成，开始复制文件到服务端...

:: 复制文件到服务端 覆盖旧文件
copy /Y 7zrpw.exe .\server\downloads\7zrpw.exe
copy /Y 7zrpw.exe .\server\downloads\7zrpw_v0.1.5.exe

echo 复制完成！
