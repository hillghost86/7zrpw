package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 定义文件大小阈值(10MB)
const FILE_SIZE_THRESHOLD = 10 * 1024 * 1024

// 函数说明：读取密码文件
// 参数：
// path: 密码文件路径
// 返回：密码列表，密码数量，错误信息
func readPasswordFile(path string) ([]string, int, error) {
	// 获取文件信息和大小
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("获取文件信息失败: %v", err)
	}

	// 小文件直接读取
	if fileInfo.Size() <= FILE_SIZE_THRESHOLD {
		passwords, err := scanPasswords(path)
		if err != nil {
			return nil, 0, err
		}
		return passwords, len(passwords), nil
	}

	// 大文件用 goroutine 读取并加超时，避免慢盘长时间阻塞
	resultChan := make(chan []string, 1)
	errorChan := make(chan error, 1)
	go func() {
		passwords, err := scanPasswords(path)
		if err != nil {
			errorChan <- err
			return
		}
		resultChan <- passwords
	}()

	select {
	case passwords := <-resultChan:
		return passwords, len(passwords), nil
	case err := <-errorChan:
		return nil, 0, err
	case <-time.After(30 * time.Second): // 兜底：读取超时
		return nil, 0, fmt.Errorf("读取文件超时")
	}
}

// scanPasswords 逐行读取密码文件，去除空白行
func scanPasswords(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var passwords []string
	scanner := bufio.NewScanner(file)

	// 注意：设置更大的 buffer 以提高读取性能
	const maxCapacity = 512 * 1024 // 512KB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		if password := strings.TrimSpace(scanner.Text()); password != "" {
			passwords = append(passwords, password)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	return passwords, nil
}

// 函数说明：获取所有密码
func getAllPasswords() ([]string, string, error) {
	// 获取可能的密码文件路径
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	currentDir, _ := os.Getwd()

	// 按优先级顺序存储密码文件路径
	dictPaths := []string{
		filepath.Join(currentDir, "passwd.txt"), // 当前目录
		filepath.Join(exeDir, "passwd.txt"),     // 程序目录
	}

	// 对路径进行去重
	var uniquePaths []string
	seen := make(map[string]bool)

	for _, path := range dictPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if !seen[absPath] {
			seen[absPath] = true
			uniquePaths = append(uniquePaths, path)
		}
	}

	// 用map去重
	passwordMap := make(map[string]bool)
	var usedPaths []string
	filePasswords := make(map[string]int)

	// 读取所有密码文件
	for _, path := range uniquePaths {
		if passwords, count, err := readPasswordFile(path); err == nil {
			if count > 0 {
				usedPaths = append(usedPaths, path)
				filePasswords[path] = count
				// 添加密码到map中去重
				for _, password := range passwords {
					passwordMap[password] = true
				}
			}
		}
	}

	// 如果没有找到任何密码
	if len(passwordMap) == 0 {
		return nil, "", fmt.Errorf("未找到密码文件或密码为空")
	}

	// 转换map为切片
	var uniquePasswords []string
	for pass := range passwordMap {
		uniquePasswords = append(uniquePasswords, pass)
	}

	// 生成使用的密码文件信息
	var usedPathsInfo string
	if len(usedPaths) > 0 {
		usedPathsInfo = "使用的密码文件:\n"
		for i, path := range usedPaths {
			usedPathsInfo += fmt.Sprintf("%d. %s (包含 %d 个密码)\n", i+1, path, filePasswords[path])
		}
		usedPathsInfo += fmt.Sprintf("\n去重后共 %d 个密码", len(uniquePasswords))
	}

	return uniquePasswords, usedPathsInfo, nil
}

// 函数说明：保存密码到passwd.txt文件
// 参数：
// password: 密码
// 返回：错误信息
func savePasswordToFile(password string) error {
	// 获取程序路径
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %v", err)
	}

	// 构建密码文件路径
	passwdPath := filepath.Join(filepath.Dir(exePath), "passwd.txt")

	// 检查密码是否已存在
	content, err := os.ReadFile(passwdPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取密码文件失败: %v", err)
	}

	// 如果文件存在，检查密码是否已在文件中（按行精确匹配）
	if err == nil {
		// 将内容分割成行
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			// 去除每行前后的空白字符
			trimmedLine := strings.TrimSpace(line)
			// 精确匹配整行
			if trimmedLine == password {
				return nil
			}
		}
	}

	// 以追加模式打开文件
	f, err := os.OpenFile(passwdPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开密码文件失败: %v", err)
	}
	defer f.Close()

	// 如果文件不为空且最后一个字符不是换行符，先写入换行符
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return fmt.Errorf("写入换行符失败: %v", err)
		}
	}

	// 只追加新密码
	if _, err := f.WriteString(password + "\n"); err != nil {
		return fmt.Errorf("写入密码失败: %v", err)
	}

	return nil
}
