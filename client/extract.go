package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// 函数说明：处理解压文件
// 参数：
// archivePath: 压缩文件路径
// extractPath: 解压路径
// password: 密码
// isFound: 是否找到密码
func handleExtract(archivePath string, extractPath string, password string, isFound bool) {
	if isFound {
		if password == "" {
			fmt.Println("\n文件无密码")
		} else {
			fmt.Printf("\n找到正确密码: [%s]\n", password)
		}
	} else {
		fmt.Printf("\n密码正确: [%s]\n", password)
	}

	fmt.Println("正在解压文件...")
	if err := extractArchive(archivePath, password, extractPath); err != nil {
		fmt.Printf("解压失败: %v\n", err)
	} else {
		fmt.Printf("\n解压成功！\n")
		fmt.Printf("文件已保存到: %s\n", formatPath(extractPath))
	}
}

// 函数说明：解压函数
// 参数：
// archivePath: 压缩文件路径
// password: 密码
// extractPath: 解压路径
// 返回：错误信息
func extractArchive(archivePath string, password string, extractPath string) error {

	// 如果解压目录不存在，则创建解压目录
	if _, err := os.Stat(extractPath); os.IsNotExist(err) { // 如果解压目录不存在
		if err := os.MkdirAll(extractPath, 0755); err != nil { // 创建解压目录
			return fmt.Errorf("创建解压目录失败: %v", err) //
		}
	}

	sevenZPath := getSevenZipPath()

	args := []string{
		"x",
		"-y",
		format7zPasswordArg(password),
		fmt.Sprintf("-o%s", extractPath),
		archivePath,
	}

	cmd := exec.Command(sevenZPath, args...)
	done := make(chan bool)
	startTime := time.Now()

	// 启动进度显示
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				elapsed := time.Since(startTime)
				fmt.Printf("\r解压中，请稍等... 已用时: %s", formatDuration(elapsed))
				time.Sleep(time.Second)
			}
		}
	}()

	// 执行命令
	err := cmd.Run()

	// 停止进度显示
	done <- true
	fmt.Println()

	// 显示总用时
	totalTime := time.Since(startTime)
	fmt.Printf("\n解压完成，总用时: %s\n", formatDuration(totalTime))
	reportPassword(archivePath, password)

	if err != nil {
		return fmt.Errorf("解压失败: %v", err)
	}

	return nil
}

// 处理密码破解失败的情况
// 参数：
// archivePath: 压缩文件路径
// extractPath: 解压路径
// reader: 输入读取器（用于读取含空格的密码）
func handleCrackFailed(archivePath string, extractPath string, reader *bufio.Reader) {
	fmt.Println("\n密码破解失败！")

	for {
		fmt.Print("请输入新的密码，右键直接粘贴(直接回车退出): ")
		var password string

		// 检测右键点击
		state, _, _ := getAsyncKeyState.Call(uintptr(VK_CONTROL))
		if state&0x8000 != 0 {
			// 如果按下了 Ctrl，等待右键点击
			time.Sleep(100 * time.Millisecond)
			state, _, _ = getAsyncKeyState.Call(uintptr(WM_RBUTTONDOWN))
			if state&0x8000 != 0 {
				// 获取剪贴板内容
				password = getClipboardText()
				fmt.Println(password) // 显示粘贴的内容
			}
		} else {
			// 普通输入（使用 readLineInput 支持密码中含空格）
			password = readLineInput(reader)
		}

		if password == "" {
			return
		}

		if testPassword(archivePath, password) {
			handleExtract(archivePath, extractPath, password, true)
			//保存密码到passwd.txt文件
			if err := savePasswordToFile(password); err != nil {
				fmt.Printf("保存密码失败: %v\n", err)
			} else {
				fmt.Printf("新密码【%s】已保存到passwd.txt文件。 \n", password)
			}
			return
		} else {
			fmt.Println("\n密码错误！请重试或回车退出")
		}
	}
}

//	函数说明：处理压缩文件
//
// 参数：
// archivePath: 压缩文件路径
// passwords: 密码列表
// passwordsInfo: 使用的密码文件信息
// reader: 输入读取器（用于密码输入等，可为 nil）
func processArchive(archivePath string, passwords []string, passwordsInfo string, reader *bufio.Reader) {
	// 获取文件信息
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		fmt.Printf("无法获取文件信息: %v\n", err)
		return
	}

	fmt.Printf("正在处理文件: %s\n", formatPath(archivePath))
	fmt.Printf("文件大小: %s\n", formatFileSize(fileInfo.Size()))

	// 检查文件类型并显示
	fileType := getFileType(archivePath)
	fmt.Printf("文件类型: %s\n", getFileTypeDesc(fileType))

	// 获取解压路径
	extractPath := getDefaultExtractPath(archivePath)

	// 获取第一个分卷
	firstVolume, err := getFirstVolumePath(archivePath)
	if err != nil {
		fmt.Printf("\n%v\n", err)
		return
	}

	if firstVolume != archivePath {
		fmt.Printf("使用第一个分卷: %s\n", firstVolume)
		archivePath = firstVolume
	}

	// 检查是否需要密码
	if !isPasswordRequired(fileType) {
		fmt.Println("检测到无需密码的文件格式，直接解压...")
		if err := extractArchive(archivePath, "", extractPath); err != nil {
			fmt.Printf("解压失败: %v\n", err)
		} else {
			fmt.Printf("\n解压成功！\n")
			fmt.Printf("文件已保存到: %s\n", formatPath(extractPath))
		}
		return
	}

	// 需要密码的文件处理逻辑
	if len(passwords) > 0 {
		fmt.Println(passwordsInfo)
	}
	fmt.Println("\n开始尝试破解...")

	// 尝试使用找到的密码解压
	if foundPassword, err := crackArchive(archivePath, passwords); err == nil {
		handleExtract(archivePath, extractPath, foundPassword, true)
	} else {
		if reader != nil {
			handleCrackFailed(archivePath, extractPath, reader)
		} else {
			handleCrackFailed(archivePath, extractPath, bufio.NewReader(os.Stdin))
		}
	}
}
