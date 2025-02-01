package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// UpdateManager 更新管理器
type UpdateManager struct {
	CurrentVersion string
}

// NewUpdateManager 创建更新管理器
func NewUpdateManager(version string) (*UpdateManager, error) {
	manager := &UpdateManager{
		CurrentVersion: version,
	}
	return manager, nil
}

// CheckUpdate 检查更新
func (m *UpdateManager) CheckUpdate(force bool) error {
	fmt.Printf("当前版本: %s\n", m.CurrentVersion)

	// 从服务器获取版本信息
	resp, err := http.Get("http://127.0.0.1:8080/api/v1/version")
	if err != nil {
		return fmt.Errorf("无法连接到更新服务器，请稍后重试")
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新服务器暂时不可用 (HTTP %d)", resp.StatusCode)
	}

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取服务器响应失败")
	}

	// 解析版本信息
	var info VersionInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("解析版本信息失败")
	}

	// 验证服务器返回的数据
	if info.Version == "" || info.DownloadURL == "" {
		return fmt.Errorf("服务器返回的版本信息不完整")
	}

	// 判断是否需要更新
	hasNewVersion := info.Version != m.CurrentVersion
	needUpdate := hasNewVersion || force

	// 如果版本相同且是服务器强制更新，则忽略
	if !hasNewVersion && info.ForceUpdate && !force {
		return nil
	}

	if !needUpdate {
		fmt.Println("当前已是最新版本")
		return nil
	}

	// 显示更新信息
	if hasNewVersion {
		fmt.Printf("\n发现新版本: %s\n", info.Version)
		if info.MD5 != "" {
			fmt.Printf("新版本MD5: %s\n", info.MD5)
		}
	} else if force {
		fmt.Printf("\n准备重新安装当前版本: %s\n", info.Version)
	}

	// 显示更新说明
	if info.ReleaseNote != "" {
		fmt.Printf("更新说明: \n%s\n", info.ReleaseNote)
	}

	// 显示手动下载地址
	fmt.Printf("手动下载地址: %s\n", info.DownloadURL)

	// 执行更新
	if info.ForceUpdate && hasNewVersion {
		fmt.Println("\n这是一个强制更新，系统将自动执行更新...")
		return m.doUpdate(info)
	}

	// 询问用户是否更新
	fmt.Print("\n是否立即更新? (y/n) [Y]: ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("读取用户输入失败")
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	// 如果用户直接回车或输入 y/yes，则执行更新
	if answer == "" || answer == "y" || answer == "yes" {
		return m.doUpdate(info)
	}

	// 用户选择不更新，直接返回
	return nil
}

// doUpdate 执行更新
func (m *UpdateManager) doUpdate(info VersionInfo) error {
	fmt.Println("\n=== 开始更新过程 ===")

	// 获取当前程序路径
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %v", err)
	}

	// 下载新版本
	fmt.Printf("正在下载新版本: %s\n", info.DownloadURL)
	resp, err := http.Get(info.DownloadURL)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()

	// 创建临时文件
	tempFile := filepath.Join(filepath.Dir(exePath), "update.tmp")
	out, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}

	// 获取文件大小
	fileSize := resp.ContentLength
	fmt.Printf("文件大小: %.2f MB\n", float64(fileSize)/1024/1024)

	// 下载文件并显示进度
	done := make(chan bool)
	go func() {
		var downloaded int64
		buffer := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				_, writeErr := out.Write(buffer[:n])
				if writeErr != nil {
					fmt.Printf("\n写入失败: %v\n", writeErr)
					break
				}
				downloaded += int64(n)
				if fileSize > 0 {
					progress := float64(downloaded) / float64(fileSize) * 100
					fmt.Printf("\r下载进度: %.1f%% (%.2f/%.2f MB)",
						progress,
						float64(downloaded)/1024/1024,
						float64(fileSize)/1024/1024)
				}
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	<-done
	fmt.Println("\n下载完成")

	// 关闭输出文件
	out.Close()

	// 验证MD5
	if info.MD5 != "" {
		fmt.Println("正在验证文件完整性...")
		md5sum, err := calculateMD5(tempFile)
		if err != nil {
			os.Remove(tempFile)
			return fmt.Errorf("验证文件失败: %v", err)
		}
		if md5sum != info.MD5 {
			os.Remove(tempFile)
			return fmt.Errorf("文件校验失败，可能已损坏")
		}
		fmt.Println("文件验证通过")
	}

	// 在创建批处理文件前输出最后的确认信息
	fmt.Printf("\n=== 更新确认 ===\n")
	fmt.Printf("更新文件已下载和验证\n")
	fmt.Printf("即将从版本 %s 更新到 %s\n", m.CurrentVersion, info.Version)

	// 显示3秒倒计时
	fmt.Println("\n程序将在3秒后开始更新...")
	for i := 3; i > 0; i-- {
		fmt.Printf("\r倒计时: %d 秒", i)
		time.Sleep(time.Second)
	}
	fmt.Println("\n开始更新...")

	// 创建批处理文件
	batContent := fmt.Sprintf(`@echo off
chcp 936 >nul
title Update
echo Waiting for program to exit...
timeout /t 1 /nobreak > nul

:kill_loop
taskkill /f /im "%s" >nul 2>&1
timeout /t 1 /nobreak > nul
tasklist | find /i "%s" >nul
if not errorlevel 1 goto kill_loop

echo Creating backup...
copy /y "%s" "%s.bak" >nul
if errorlevel 1 (
    echo Backup failed!
    exit /b 1
)

echo Updating program...
copy /y "%s" "%s" >nul
if errorlevel 1 (
    echo Update failed! Restoring...
    copy /y "%s.bak" "%s" >nul
    del "%s"
    echo Previous version restored
    exit /b 1
)

echo Cleaning up...
if exist "%s.bak" del "%s.bak" >nul
if exist "%s" del "%s" >nul

echo Update completed!
start "" "%s"
exit
`, filepath.Base(exePath), filepath.Base(exePath),
		exePath, exePath,
		tempFile, exePath,
		exePath, exePath, tempFile,
		exePath, exePath,
		tempFile, tempFile,
		exePath)

	// 创建更新脚本
	batPath := filepath.Join(filepath.Dir(exePath), "update.bat")
	if err := os.WriteFile(batPath, []byte(batContent), 0644); err != nil {
		return fmt.Errorf("创建更新脚本失败: %v", err)
	}

	// 使用新的 API 启动更新进程
	batPathPtr, err := windows.UTF16PtrFromString(batPath)
	if err != nil {
		return fmt.Errorf("转换路径失败: %v", err)
	}

	dirPtr, err := windows.UTF16PtrFromString(filepath.Dir(batPath))
	if err != nil {
		return fmt.Errorf("转换目录失败: %v", err)
	}

	var startupInfo windows.StartupInfo
	var processInfo windows.ProcessInformation

	err = windows.CreateProcess(
		nil,          // 应用程序名称
		batPathPtr,   // 命令行
		nil,          // 进程安全属性
		nil,          // 线程安全属性
		false,        // 是否继承句柄
		0,            // 创建标志（移除 CREATE_NO_WINDOW）
		nil,          // 环境变量
		dirPtr,       // 当前目录
		&startupInfo, // 启动信息
		&processInfo, // 进程信息
	)

	if err != nil {
		return fmt.Errorf("启动更新进程失败: %v", err)
	}

	// 关闭进程和线程句柄
	windows.CloseHandle(processInfo.Thread)
	windows.CloseHandle(processInfo.Process)

	// 直接退出程序前的最后提示
	fmt.Println("\n=== 更新中，请稍等 ===")

	os.Exit(0)
	return nil
}

// 添加 calculateMD5 函数
func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算MD5失败: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
