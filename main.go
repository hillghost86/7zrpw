package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/h2non/filetype"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

//go:embed resources/7z.exe
var sevenZipExe []byte

//go:embed resources/7z.dll
var sevenZipDll []byte

// 定义压缩文件的基本结构
type ArchiveEntry struct {
	Name      string
	CRC32     uint32
	Size      uint64
	Encrypted bool
}

// 定义常量和结构
const (
	// ZIP格式相关常量
	ZIP_LOCAL_HEADER_MAGIC   = 0x04034b50
	ZIP_CENTRAL_HEADER_MAGIC = 0x02014b50
	ZIP_END_HEADER_MAGIC     = 0x06054b50

	// 7Z格式相关常量
	SEVEN_ZIP_MAGIC = "7z\xBC\xAF\x27\x1C"
)

type ZipHeader struct {
	Magic       uint32
	Version     uint16
	Flags       uint16
	Method      uint16
	ModTime     uint16
	ModDate     uint16
	CRC32       uint32
	CompSize    uint32
	UncompSize  uint32
	NameLength  uint16
	ExtraLength uint16
}

func init() {
	// 确保临时目录存在
	tempDir := filepath.Join(os.TempDir(), "7zrpw")
	os.MkdirAll(tempDir, 0755)

	// 提取7z.exe到临时目录
	sevenZipPath := filepath.Join(tempDir, "7z.exe")
	if _, err := os.Stat(sevenZipPath); os.IsNotExist(err) {
		err = os.WriteFile(sevenZipPath, sevenZipExe, 0755)
		if err != nil {
			panic(fmt.Sprintf("无法释放7z.exe: %v", err))
		}
	}

	// 提取7z.dll到临时目录
	dllPath := filepath.Join(tempDir, "7z.dll")
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		err = os.WriteFile(dllPath, sevenZipDll, 0755)
		if err != nil {
			panic(fmt.Sprintf("无法释放7z.dll: %v", err))
		}
	}
}

func getSevenZipPath() string {
	return filepath.Join(os.TempDir(), "7zrpw", "7z.exe")
}

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

// 支持的文件类型
const (
	TYPE_ZIP      = iota // .zip
	TYPE_RAR             // .rar
	TYPE_7Z              // .7z
	TYPE_ZIP_PART        // .zip.001, .z01 等分卷
	TYPE_RAR_PART        // .part1.rar, .r01 等分卷
	TYPE_GZ              // .gz, .tar.gz, .tgz
	TYPE_BZ2             // .bz2, .tar.bz2, .tbz2
	TYPE_TAR             // .tar
	TYPE_TAR_PART        // .tar.001, .tar.002 等分卷
	TYPE_XZ              // .xz, .tar.xz, .txz
	TYPE_CAB             // .cab
	TYPE_ISO             // .iso
	TYPE_ARJ             // .arj
	TYPE_LZH             // .lzh, .lha
	TYPE_WIM             // .wim, .swm (分段 WIM)
)

// 获取文件类型
func getFileType(path string) int {
	// 首先检查分卷格式（通过文件名）
	baseName := strings.ToLower(filepath.Base(path))
	if strings.Contains(baseName, ".zip.") || strings.HasSuffix(baseName, ".z01") {
		return TYPE_ZIP_PART
	}
	if strings.Contains(baseName, ".part") && strings.HasSuffix(baseName, ".rar") ||
		strings.HasSuffix(baseName, ".r01") {
		return TYPE_RAR_PART
	}
	if matched, _ := regexp.MatchString(`\.tar\.\d{3}$`, baseName); matched {
		return TYPE_TAR_PART
	}

	// 读取文件头
	buf, err := os.ReadFile(path)
	if err != nil {
		return -1
	}

	// 使用 filetype 库检测文件类型
	kind, err := filetype.Match(buf)
	if err == nil && kind != filetype.Unknown {
		switch kind.MIME.Value {
		case "application/zip":
			return TYPE_ZIP
		case "application/x-rar-compressed":
			return TYPE_RAR
		case "application/x-7z-compressed":
			return TYPE_7Z
		case "application/gzip":
			return TYPE_GZ
		case "application/x-bzip2":
			return TYPE_BZ2
		case "application/x-tar":
			return TYPE_TAR
		case "application/x-xz":
			return TYPE_XZ
		case "application/vnd.ms-cab-compressed":
			return TYPE_CAB
		case "application/x-iso9660-image":
			return TYPE_ISO
		}
	}

	// 如果文件类型检测失败，回退到扩展名检测
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip":
		return TYPE_ZIP
	case ".rar":
		return TYPE_RAR
	case ".7z":
		return TYPE_7Z
	case ".gz", ".tgz":
		return TYPE_GZ
	case ".bz2", ".tbz2":
		return TYPE_BZ2
	case ".tar":
		return TYPE_TAR
	case ".xz", ".txz":
		return TYPE_XZ
	case ".cab":
		return TYPE_CAB
	case ".iso":
		return TYPE_ISO
	case ".arj":
		return TYPE_ARJ
	case ".lzh", ".lha":
		return TYPE_LZH
	case ".wim", ".swm":
		return TYPE_WIM
	}

	// 检查特殊格式（如 .tar.gz）
	if strings.HasSuffix(baseName, ".tar.gz") {
		return TYPE_GZ
	}
	if strings.HasSuffix(baseName, ".tar.bz2") {
		return TYPE_BZ2
	}
	if strings.HasSuffix(baseName, ".tar.xz") {
		return TYPE_XZ
	}

	return -1
}

func testPassword(archivePath, password string) bool {
	// 构建测试命令
	args := []string{
		"t",             // 测试命令
		"-mmt=on",       // 启用多线程
		"-p" + password, // 密码
		archivePath,     // 文件路径
	}

	// 创建命令
	cmd := exec.Command(getSevenZipPath(), args...)
	cmd.Env = append(os.Environ(), "LANG=C.UTF-8")

	// 创建结果通道
	resultChan := make(chan bool, 1)

	// 启动命令并检查输出
	go func() {
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)

		// 检查成功标记
		if strings.Contains(outputStr, "Everything is Ok") {
			resultChan <- true
			return
		}

		// 检查错误标记
		if strings.Contains(outputStr, "Cannot open encrypted archive") ||
			strings.Contains(outputStr, "Headers Error") ||
			strings.Contains(outputStr, "Can't open as archive") ||
			strings.Contains(outputStr, "ERROR: Wrong password") ||
			strings.Contains(outputStr, "Archives with Errors") {
			resultChan <- false
			return
		}
		resultChan <- true
	}()

	// 等待结果或超时
	select {
	case result := <-resultChan:
		return result
	case <-time.After(2 * time.Second):
		// 2秒内没有错误标记，说明是正确密码
		cmd.Process.Kill()
		return true
	}
}

// 格式化进度显示
func formatProgress(current, total int, currentPass string) string {
	percent := float64(current) * 100.0 / float64(total)
	// 先清除整行，再显示新内容
	return fmt.Sprintf("\r%s\r正在尝试密码... %d/%d (%.1f%%) [%s]",
		strings.Repeat(" ", 100), // 清除整行
		current, total, percent, currentPass)
}

func crackArchive(archivePath string, passwords []string) (string, error) {
	// 首先尝试空密码
	if testPassword(archivePath, "") {
		return "", nil
	}

	// 逐个尝试密码
	for i, pass := range passwords {
		// 显示进度条
		fmt.Print(formatProgress(i+1, len(passwords), pass))

		// 测试密码
		if testPassword(archivePath, pass) {
			fmt.Print("\r" + strings.Repeat(" ", 100) + "\r")
			return pass, nil
		}
	}

	return "", fmt.Errorf("未找到正确密码")
}

func extractArchive(archivePath, password string) error {

	// 构建解压命令
	args := []string{
		"x",             // 解压命令
		"-y",            // 自动确认
		"-aoa",          // 覆盖已存在的文件
		"-p" + password, // 密码
		archivePath,     // 文件路径
	}

	// 创建命令
	cmd := exec.Command(getSevenZipPath(), args...)
	cmd.Env = append(os.Environ(), "LANG=C.UTF-8")

	// 直接执行命令
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("解压失败: %v", err)
	}

	fmt.Println("解压完成！")
	return nil
}

func findCompressFiles(dir string) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}

	// 定义常见压缩文件扩展名
	extensions := []string{
		".zip", ".rar", ".7z",
		".gz", ".tgz", ".tar.gz",
		".bz2", ".tbz2", ".tar.bz2",
		".tar", ".xz", ".txz", ".tar.xz",
		".cab", ".iso", ".arj",
		".lzh", ".lha",
	}

	// 检查分卷格式的正则表达式
	partPatterns := []string{
		`\.zip\.\d+$`, `\.z\d{2}$`,
		`\.part\d+\.rar$`, `\.r\d{2}$`,
		`\.7z\.\d+$`,
		`\.tar\.\d{3}$`, // 添加对 .tar.001 等分卷的支持
	}

	// 分别存储压缩文件和目录
	var compressFiles []string
	var directories []string

	for _, entry := range entries {
		// 获取原始文件名
		rawName := entry.Name()

		// 尝试解码
		decodedName := decodeGBK(rawName)

		// 用于匹配的小写名称
		lowerName := strings.ToLower(decodedName)

		if entry.IsDir() {
			directories = append(directories, decodedName+"/")
			continue
		}

		// 检查是否是压缩文件
		isCompressFile := false

		// 检查常规扩展名
		for _, ext := range extensions {
			if strings.HasSuffix(lowerName, ext) {
				compressFiles = append(compressFiles, decodedName)
				isCompressFile = true
				break
			}
		}

		// 如果不是常规压缩文件，检查分卷格式
		if !isCompressFile {
			for _, pattern := range partPatterns {
				matched, _ := regexp.MatchString(pattern, lowerName)
				if matched {
					compressFiles = append(compressFiles, decodedName)
					break
				}
			}
		}
	}

	// 分别排序
	sort.Strings(compressFiles)
	sort.Strings(directories)

	// 按顺序合并：压缩文件在前，目录在后
	files = append(files, compressFiles...)
	files = append(files, directories...)

	return files
}

func readPasswordFile(path string) ([]string, error) {
	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取密码文件失败: %v", err)
	}

	// 检测并处理文件编码
	decodedContent := decodeGBK(string(content))

	// 分割为行
	lines := strings.Split(decodedContent, "\n")
	var passwords []string

	// 处理每一行
	for _, line := range lines {
		// 移除 BOM 标记
		line = strings.TrimPrefix(line, "\ufeff")
		// 移除 Windows 的回车符和空白字符
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))

		if line != "" {
			passwords = append(passwords, line)
		}
	}

	return passwords, nil
}

func installContext() error {
	// 获取程序路径
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %v", err)
	}
	exePath := strings.ReplaceAll(exe, "/", "\\")

	// 创建注册表项
	k, _, err := registry.CreateKey(
		registry.CLASSES_ROOT,
		`*\shell\7zrpw`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("创建注册表项失败: %v", err)
	}
	defer k.Close()

	// 设置显示名称和图标
	if err := k.SetStringValue("", "使用7zrpw解压"); err != nil {
		return fmt.Errorf("设置显示名称失败: %v", err)
	}
	if err := k.SetStringValue("Icon", exePath); err != nil {
		return fmt.Errorf("设置图标失败: %v", err)
	}

	// 创建command子项
	k2, _, err := registry.CreateKey(
		registry.CLASSES_ROOT,
		`*\shell\7zrpw\command`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("创建command子项失败: %v", err)
	}
	defer k2.Close()

	// 设置命令
	command := fmt.Sprintf("\"%s\" \"%%1\"", exePath)
	if err := k2.SetStringValue("", command); err != nil {
		return fmt.Errorf("设置命令失败: %v", err)
	}

	return nil
}

func uninstallContext() error {
	// 删除注册表项
	err := registry.DeleteKey(registry.CLASSES_ROOT, `*\shell\7zrpw\command`)
	if err != nil {
		return fmt.Errorf("删除command子项失败: %v", err)
	}
	err = registry.DeleteKey(registry.CLASSES_ROOT, `*\shell\7zrpw`)
	if err != nil {
		return fmt.Errorf("删除注册表项失败: %v", err)
	}
	return nil
}

// 添加一个辅助函数来判断是否需要密码
func isPasswordRequired(fileType int) bool {
	switch fileType {
	case TYPE_ZIP, TYPE_ZIP_PART,
		TYPE_RAR, TYPE_RAR_PART,
		TYPE_7Z,
		TYPE_ARJ,
		TYPE_LZH:
		return true
	case TYPE_TAR, TYPE_TAR_PART,
		TYPE_GZ,
		TYPE_BZ2,
		TYPE_XZ,
		TYPE_ISO,
		TYPE_WIM,
		TYPE_CAB:
		return false
	default:
		return true // 未知格式默认需要密码
	}
}

// 读取所有可能的密码文件并合并密码
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

	// 用map去重
	passwordMap := make(map[string]bool)
	var usedPaths []string
	var totalCount int

	// 读取所有密码文件
	for _, path := range dictPaths {
		if passwords, err := readPasswordFile(path); err == nil {
			usedPaths = append(usedPaths, path)
			for _, pass := range passwords {
				if !passwordMap[pass] {
					passwordMap[pass] = true
					totalCount++
				}
			}
		}
	}

	// 如果没有找到任何密码文件
	if len(passwordMap) == 0 {
		return nil, "", fmt.Errorf("未找到密码文件")
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
			passwords, err := readPasswordFile(path)
			if err != nil {
				usedPathsInfo += fmt.Sprintf("%d. %s (读取失败: %v)\n", i+1, path, err)
			} else {
				usedPathsInfo += fmt.Sprintf("%d. %s (包含 %d 个密码)\n", i+1, path, len(passwords))
			}
		}
		usedPathsInfo += fmt.Sprintf("\n去重后共 %d 个密码", len(uniquePasswords))
	}

	return uniquePasswords, usedPathsInfo, nil
}

// 修改 processArchive 函数
func processArchive(archivePath string, passwords []string, passwordsInfo string) {
	fmt.Printf("正在处理文件: %s\n", filepath.Base(archivePath))

	// 检查文件类型
	fileType := getFileType(archivePath)

	// 检查是否需要密码
	if !isPasswordRequired(fileType) {
		fmt.Println("检测到无需密码的文件格式，直接解压...")
		if err := extractArchive(archivePath, ""); err != nil {
			fmt.Printf("解压失败: %v\n", err)
		} else {
			fmt.Println("解压完成！")
		}
		return
	}

	// 需要密码的文件处理逻辑
	if len(passwords) > 0 {
		fmt.Println(passwordsInfo) // 显示密码文件信息
	}
	fmt.Println("\n开始尝试破解...")

	// 尝试破解
	if pass, err := crackArchive(archivePath, passwords); err == nil {
		if pass == "" {
			fmt.Println("\n文件无密码")
		} else {
			fmt.Printf("\n找到正确密码: [%s]\n", pass)
		}
		fmt.Println("正在解压文件...")
		if err := extractArchive(archivePath, pass); err != nil {
			fmt.Printf("解压失败: %v\n", err)
		}
	} else {
		// 自动破解失败，请手动输入密码
		for {
			fmt.Printf("\n密码测试失败，请手动输入密码（直接回车退出）: ")
			var manualPass string
			fmt.Scanln(&manualPass)
			if manualPass == "" {
				break
			}

			fmt.Println("正在测试密码...")
			if testPassword(archivePath, manualPass) {
				fmt.Printf("\n找到正确密码: [%s]\n", manualPass)
				fmt.Println("正在解压文件...")
				if err := extractArchive(archivePath, manualPass); err != nil {
					fmt.Printf("解压失败: %v\n", err)
				} else {
					break
				}
			} else {
				fmt.Println("密码错误，请重试")
			}
		}

	}

}

func main() {
	// 检查命令行参数
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--install":
			if err := installContext(); err != nil {
				fmt.Printf("安装右键菜单失败: %v\n", err)
			} else {
				fmt.Println("右键菜单安装成功！")
			}
			fmt.Print("\n按回车键退出...")
			fmt.Scanln()
			return
		case "--uninstall":
			if err := uninstallContext(); err != nil {
				fmt.Printf("卸载右键菜单失败: %v\n", err)
			} else {
				fmt.Println("右键菜单卸载成功！")
			}
			fmt.Print("\n按回车键退出...")
			fmt.Scanln()
			return
		default:
			// 如果参数是文件路径，直接处理该文件
			filePath := os.Args[1]
			if _, err := os.Stat(filePath); err == nil {
				// 切换到文件所在目录
				dir := filepath.Dir(filePath)
				os.Chdir(dir)

				// 获取文件名
				fileName := filepath.Base(filePath)

				// 在处理文件之前获取密码
				passwords, passwordsInfo, err := getAllPasswords()
				if err != nil {
					fmt.Printf("\n提示：%v\n", err)
					fmt.Println("将尝试空密码，如果失败可以手动输入密码")
					passwords = []string{}
					passwordsInfo = ""
				}

				// 处理文件
				if getFileType(fileName) != -1 {
					processArchive(fileName, passwords, passwordsInfo)
				} else {
					fmt.Println("不支持的文件格式")
				}
				fmt.Print("\n按回车键退出...")
				fmt.Scanln()
				return
			}
		}
	}

	// 原有的交互式处理逻辑
	currentDir := "."
	for {
		var archivePath string
		var dictPath string = "passwd.txt"

		// 检查当前目录下的文件和目录
		files := findCompressFiles(currentDir)
		fmt.Printf("\n当前目录: %s\n", currentDir)
		if len(files) > 0 {
			fmt.Println("\n发现以下文件和目录：")
			// 先显示压缩文件
			for i, file := range files {
				fmt.Printf("%d: %s\n", i+1, file)
			}
			// 然后显示其他选项
			fmt.Println("\n0: 退出程序")
			fmt.Println("a: 解压所有压缩文件")
			fmt.Println("b: 返回上级目录")

			fmt.Print("\n请选择 (输入序号或路径): ")
			var choice string
			fmt.Scanln(&choice)

			if choice == "0" {
				fmt.Println("程序已退出")
				return
			} else if choice == "b" || choice == "B" {
				// 返回上级目录
				parent := filepath.Dir(currentDir)
				if parent != currentDir {
					currentDir = parent
				}
				continue
			} else if choice == "a" || choice == "A" {
				// 处理所有压缩文件
				fmt.Println("\n开始处理所有压缩文件...")

				// 获取压缩文件列表（不包括目录）
				var compressFiles []string
				for _, file := range files {
					if !strings.HasSuffix(file, "/") {
						compressFiles = append(compressFiles, filepath.Join(currentDir, file))
					}
				}

				if len(compressFiles) == 0 {
					fmt.Println("当前目录没有压缩文件")
					continue
				}

				// 获取所有密码
				passwords, passwordsInfo, err := getAllPasswords()
				if err != nil {
					fmt.Printf("\n提示：%v\n", err)
					fmt.Println("将尝试空密码，如果失败可以手动输入密码")
					passwords = []string{}
					passwordsInfo = ""
				}

				fmt.Printf("共 %d 个密码\n\n", len(passwords))
				fmt.Println("开始尝试破解...")

				// 处理每个压缩文件
				for i, file := range compressFiles {
					fmt.Printf("\n[%d/%d] 处理文件: %s\n", i+1, len(compressFiles), filepath.Base(file))
					processArchive(file, passwords, passwordsInfo)
				}

				fmt.Print("\n按回车键继续...")
				fmt.Scanln()
				continue
			}

			// 尝试解析数字选择
			if num, err := strconv.Atoi(choice); err == nil && num > 0 && num <= len(files) {
				selected := files[num-1]
				fullPath := filepath.Join(currentDir, strings.TrimSuffix(selected, "/"))

				if strings.HasSuffix(selected, "/") {
					// 如果选择的是目录，进入该目录
					currentDir = fullPath
					continue
				} else {
					// 如果选择的是文件，处理该文件
					archivePath = fullPath
				}
			} else {
				// 如果不是数字，检查是否是目录或文件路径
				fileInfo, err := os.Stat(choice)
				if err == nil {
					if fileInfo.IsDir() {
						// 如果是目录，进入该目录
						currentDir = choice
						continue
					} else {
						// 如果是文件，处理该文件
						archivePath = choice
					}
				} else {
					fmt.Printf("无效的路径: %s\n", choice)
					continue
				}
			}
		} else {
			fmt.Println("\n当前目录为空")
			fmt.Println("0: 退出程序")
			fmt.Println("b: 返回上级目录")
			fmt.Print("\n请选择或输入路径: ")
			var choice string
			fmt.Scanln(&choice)

			if choice == "0" {
				fmt.Println("程序已退出")
				return
			} else if choice == "b" || choice == "B" {
				parent := filepath.Dir(currentDir)
				if parent != currentDir {
					currentDir = parent
				}
				continue
			} else if choice != "" {
				archivePath = choice
			} else {
				continue
			}
		}

		// 检查密码文件
		if _, err := os.Stat(dictPath); os.IsNotExist(err) {
			fmt.Printf("未找到默认密码文件 %s，请输入密码文件路径 (直接回车退出): ", dictPath)
			fmt.Scanln(&dictPath)
			if dictPath == "" {
				fmt.Println("程序已退出")
				return
			}
		}

		// 检查文件是否存在
		if _, err := os.Stat(archivePath); os.IsNotExist(err) {
			fmt.Println("压缩文件不存在")
			continue
		}
		if _, err := os.Stat(dictPath); os.IsNotExist(err) {
			fmt.Println("密码文件不存在")
			continue
		}

		// 检查文件类型
		if getFileType(archivePath) == -1 {
			fmt.Println("不支持的文件格式，仅支持以下格式：")
			fmt.Println("- ZIP (.zip 及其分卷)")
			fmt.Println("- RAR (.rar 及其分卷)")
			fmt.Println("- 7Z (.7z)")
			fmt.Println("- GZ (.gz, .tar.gz, .tgz)")
			fmt.Println("- BZ2 (.bz2, .tar.bz2, .tbz2)")
			fmt.Println("- TAR (.tar)")
			fmt.Println("- XZ (.xz, .tar.xz, .txz)")
			fmt.Println("- CAB (.cab)")
			fmt.Println("- ISO (.iso)")
			fmt.Println("- ARJ (.arj)")
			fmt.Println("- LZH (.lzh, .lha)")
			continue
		}

		// 获取所有密码
		passwords, passwordsInfo, err := getAllPasswords()
		if err != nil {
			fmt.Printf("警告: %v\n", err)
			passwords = []string{}
			passwordsInfo = ""
		}

		fmt.Printf("共 %d 个密码\n\n", len(passwords))
		fmt.Println("开始尝试破解...")

		processArchive(archivePath, passwords, passwordsInfo)

		fmt.Print("\n按回车键继续...")
		fmt.Scanln()
	}
}
