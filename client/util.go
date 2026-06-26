package main

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// 函数说明：GBK解码
// 参数：
// s: 需要解码的字符串
// 返回：解码后的字符串
func decodeGBK(s string) string {
	// 如果字符串已经是UTF-8，直接返回
	if utf8.ValidString(s) {
		return s
	}

	// 尝试GBK解码
	reader := transform.NewReader(strings.NewReader(s), simplifiedchinese.GBK.NewDecoder())
	d, e := io.ReadAll(reader)
	if e != nil {
		return s
	}
	return string(d)
}

// 函数说明：格式化持续时间
// 参数：
// d: 持续时间
// 返回：格式化后的持续时间字符串
func formatDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%d时%02d分%02d秒", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%d分%02d秒", m, s)
	}
	return fmt.Sprintf("%d秒", s)
}

// 函数说明：格式化文件大小
// 参数：
// size: 文件大小
// 返回：文件大小描述
func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// 函数说明：读取一行输入（支持路径中含空格，如拖入文件时的路径）
// 参数：
// reader: 输入读取器
// 返回：去除首尾空白和引号后的输入内容
func readLineInput(reader *bufio.Reader) string {
	line, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(line)
	// 移除 Windows 拖入路径时可能添加的引号
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s
}

// 函数说明：格式化路径显示
// 参数：
// path: 路径
// 返回：格式化后的路径
func formatPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Sprintf("%s (无法获取完整路径)", path)
	}
	return absPath
}
