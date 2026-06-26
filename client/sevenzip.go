package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed resources/7z.exe
var sevenZipExe []byte

//go:embed resources/7z.dll
var sevenZipDll []byte

// 辅助函数：仅在文件不存在时提取文件（版本子目录保证更新后使用新文件）
func extractFileIfNotExist(path string, data []byte) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, data, 0755)
	}
	return nil
}

// 获取 7z 临时目录（按版本号分子目录，更新后自动使用新版本 7z.exe/dll）
func get7zTempDir() string {
	return filepath.Join(os.TempDir(), "7zrpw", "7zrpw_"+VERSION)
}

// 获取7z.exe路径
func getSevenZipPath() string {
	return filepath.Join(get7zTempDir(), "7z.exe")
}

// cleanOld7zVersionDirs 清理旧版本目录，只保留当前版本和上一个版本
func cleanOld7zVersionDirs() {
	baseDir := filepath.Join(os.TempDir(), "7zrpw")
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}

	prefix := "7zrpw_"
	var versionDirs []struct {
		name    string
		version string
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		ver := strings.TrimPrefix(entry.Name(), prefix)
		versionDirs = append(versionDirs, struct {
			name    string
			version string
		}{entry.Name(), ver})
	}

	if len(versionDirs) <= 2 {
		return
	}

	// 按版本号降序排序（新的在前）
	sort.Slice(versionDirs, func(i, j int) bool {
		return compareVersionStrings(versionDirs[i].version, versionDirs[j].version) > 0
	})

	// 保留前两个（当前和上一个），删除其余
	for i := 2; i < len(versionDirs); i++ {
		dirPath := filepath.Join(baseDir, versionDirs[i].name)
		os.RemoveAll(dirPath)
	}
}

// compareVersionStrings 比较版本字符串，返回 1/a>b, 0/a==b, -1/a<b
func compareVersionStrings(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	for i := 0; i < len(partsA) && i < len(partsB); i++ {
		nA, okA := strconv.Atoi(strings.TrimSpace(partsA[i]))
		nB, okB := strconv.Atoi(strings.TrimSpace(partsB[i]))
		if okA == nil && okB == nil {
			if nA > nB {
				return 1
			}
			if nA < nB {
				return -1
			}
		} else {
			if partsA[i] > partsB[i] {
				return 1
			}
			if partsA[i] < partsB[i] {
				return -1
			}
		}
	}
	if len(partsA) > len(partsB) {
		return 1
	}
	if len(partsA) < len(partsB) {
		return -1
	}
	return 0
}

// init 释放内嵌的 7z 组件并预加载 user32.dll
// 感谢 https://github.com/ShuiJu 的提点：7z.exe/7z.dll 需要时才释放，预加载 user32.dll
// 按版本号创建子目录，更新后新版本使用新目录，无需每次启动覆盖，不影响启动速度
func init() {
	base7zDir := filepath.Join(os.TempDir(), "7zrpw")
	if err := os.MkdirAll(base7zDir, 0755); err != nil {
		panic(fmt.Sprintf("无法创建临时目录: %v", err))
	}

	tempDir := get7zTempDir()
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		panic(fmt.Sprintf("无法创建临时目录: %v", err))
	}

	sevenZipPath := filepath.Join(tempDir, "7z.exe")
	dllPath := filepath.Join(tempDir, "7z.dll")

	// 提取必要文件（仅当不存在时，同一版本无需重复写入）
	if err := extractFileIfNotExist(sevenZipPath, sevenZipExe); err != nil {
		panic(fmt.Sprintf("无法释放7z.exe: %v", err))
	}

	if err := extractFileIfNotExist(dllPath, sevenZipDll); err != nil {
		panic(fmt.Sprintf("无法释放7z.dll: %v", err))
	}

	// 清理旧版本目录，只保留当前版本和上一个版本
	cleanOld7zVersionDirs()

	// 预加载 DLL
	if err := user32.Load(); err != nil {
		panic(fmt.Sprintf("无法加载user32.dll: %v", err))
	}
}
