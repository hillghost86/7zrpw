package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// 7z 输出中代表「密码错误 / 无法作为压缩包打开」的标记，集中维护便于增删
var sevenZipErrorMarkers = []string{
	"Cannot open encrypted archive",
	"Data Error in encrypted file",
	"ERROR:",
	"Can't open as archive",
	"ERROR: Wrong password",
	"Wrong password",
	"Archives with Errors",
}

// format7zPasswordArg 格式化 7z 的密码参数，支持含空格、引号、反斜杠等特殊字符
func format7zPasswordArg(password string) string {
	if password == "" {
		return "-p"
	}
	if strings.ContainsAny(password, " \"\\") {
		escaped := strings.ReplaceAll(password, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return "-p\"" + escaped + "\""
	}
	return "-p" + password
}

/*
*
函数说明：测试密码
参数：
archivePath: 压缩文件路径
password: 密码
返回：是否成功
*/
func testPassword(archivePath, password string) bool {
	// 构建测试命令，使用7z的t命令测试文件完整性，来判断密码是否正确
	args := []string{
		"t",
		format7zPasswordArg(password),
		archivePath,
	}

	cmd := exec.Command(getSevenZipPath(), args...)
	cmd.Env = append(os.Environ(), "LANG=C.UTF-8")

	resultChan := make(chan bool, 1)

	go func() {
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)

		// 如果输出包含以下信息，说明密码正确
		if strings.Contains(outputStr, "Everything is Ok") {
			resultChan <- true
			return
		}

		// 命中任一错误标记，说明密码错误
		for _, marker := range sevenZipErrorMarkers {
			if strings.Contains(outputStr, marker) {
				resultChan <- false
				return
			}
		}

		resultChan <- true
	}()

	// 等待结果或超时
	select {
	case result := <-resultChan: // 获取结果
		return result // 返回结果
	case <-time.After(2 * time.Second):
		// 边界：2秒内没有错误标记，说明是正确密码
		// 确保检查大文件时，7z.exe会一直检查整个文件直至检查结束。
		// 当大于2秒时，基本可以认为密码正确
		if cmd.Process != nil {
			cmd.Process.Kill() // 强制终止命令
		}
		return true // 返回true
	}
}

// 函数说明：格式化进度显示
// 参数：
// current: 当前尝试的密码数量
// total: 总的密码数量
// currentPass: 当前尝试的密码
// 返回：格式化后的进度显示字符串
func formatProgress(current, total int, currentPass string) string {
	percent := float64(current) * 100.0 / float64(total)

	// 使用已有的 decodeGBK 函数处理密码字符串
	cleanPass := decodeGBK(currentPass)

	// 先清除整行，再显示新内容
	return fmt.Sprintf("\r%s\r正在尝试密码... %d/%d （%.1f%%） [%s]",
		strings.Repeat(" ", 100), // 清除整行
		current,
		total,
		percent,
		cleanPass)
}

// 函数说明：破解压缩文件
// 参数：
// archivePath: 压缩文件路径
// passwords: 密码列表
// 返回：密码，错误信息
func crackArchive(archivePath string, passwords []string) (string, error) {
	startTime := time.Now() // 记录开始时间

	// 首先尝试空密码
	if testPassword(archivePath, "") {
		elapsed := time.Since(startTime)
		fmt.Printf("\n破解用时: %s\n", formatDuration(elapsed))
		return "", nil
	}

	// 记录已尝试的密码数量
	testedCount := 0

	// 逐个尝试密码
	for i, pass := range passwords {
		testedCount++
		// 显示进度条
		fmt.Print(formatProgress(i+1, len(passwords), pass))

		// 测试密码
		if testPassword(archivePath, pass) {
			elapsed := time.Since(startTime)
			speed := float64(testedCount) / elapsed.Seconds()
			fmt.Printf("\n破解用时: %s (平均 %.1f 密码/秒)\n", formatDuration(elapsed), speed)
			return pass, nil
		}
	}

	elapsed := time.Since(startTime)
	speed := float64(testedCount) / elapsed.Seconds()
	fmt.Printf("\n破解用时: %s (平均 %.1f 密码/秒)\n", formatDuration(elapsed), speed)
	return "", fmt.Errorf("未找到正确密码")
}
