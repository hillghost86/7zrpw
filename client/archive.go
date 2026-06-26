package main

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/h2non/filetype"
)

// 预编译正则表达式，避免重复编译
var (
	re7zPart3      = regexp.MustCompile(`\.7z\.\d{3}$`)
	re7zPartN      = regexp.MustCompile(`\.7z\.\d+$`)
	reTarPart      = regexp.MustCompile(`\.tar\.\d{3}$`)
	reZipPart3     = regexp.MustCompile(`\.zip\.\d{3}$`)
	reZipPartN     = regexp.MustCompile(`\.zip\.\d+$`)
	reZ01          = regexp.MustCompile(`\.z\d{2}$`)
	rePartRar      = regexp.MustCompile(`\.part\d+\.rar$`)
	reR01          = regexp.MustCompile(`\.r\d{2}$`)
	rePartPatterns = []*regexp.Regexp{reZipPartN, reZ01, rePartRar, reR01, re7zPartN, reTarPart}
)

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

// 函数说明：获取文件类型
// 参数：
// path: 文件路径
// 返回：文件类型
func getFileType(path string) int {
	// 1. 首先检查分卷格式（通过文件名）
	baseName := strings.ToLower(filepath.Base(path))

	// 检查7z分卷（支持任意序号）
	if re7zPart3.MatchString(baseName) {
		return TYPE_7Z_PART
	}

	if strings.Contains(baseName, ".zip.") || strings.HasSuffix(baseName, ".z01") {
		return TYPE_ZIP_PART
	}
	if strings.Contains(baseName, ".part") && strings.HasSuffix(baseName, ".rar") ||
		strings.HasSuffix(baseName, ".r01") {
		return TYPE_RAR_PART
	}
	if reTarPart.MatchString(baseName) {
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

// 函数说明：获取第一个分卷的路径
// 参数：
// archivePath: 压缩文件路径
// 返回：第一个分卷的路径，错误信息
func getFirstVolumePath(archivePath string) (string, error) {
	baseName := strings.ToLower(filepath.Base(archivePath))
	baseDir := filepath.Dir(archivePath)

	// 7Z 分卷 (.7z.001, .7z.002, ...)
	if re7zPart3.MatchString(baseName) {
		baseFile := baseName[:len(baseName)-7] // 移除 .7z.NNN
		return filepath.Join(baseDir, baseFile+".7z.001"), nil
	}

	// ZIP 分卷格式1 (.zip.001, .zip.002, ...)
	if reZipPart3.MatchString(baseName) {
		baseFile := baseName[:len(baseName)-8] // 移除 .zip.NNN
		firstPart := filepath.Join(baseDir, baseFile+".zip.001")
		if _, err := os.Stat(firstPart); err == nil {
			return firstPart, nil
		}
	}

	// ZIP 分卷格式2 (.zip, .z01, .z02, ...)
	if reZ01.MatchString(baseName) {
		baseFile := baseName[:len(baseName)-4] // 移除 .zNN
		firstPart := filepath.Join(baseDir, baseFile+".zip")
		if _, err := os.Stat(firstPart); err == nil {
			return firstPart, nil
		}
	}

	// RAR 分卷 (.part1.rar, .part2.rar, ...)
	if rePartRar.MatchString(baseName) {
		baseFile := strings.Split(baseName, ".part")[0]
		return filepath.Join(baseDir, baseFile+".part1.rar"), nil
	}

	// RAR 旧格式分卷 (.r01, .r02, ...)
	if reR01.MatchString(baseName) {
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
			for _, re := range rePartPatterns {
				if re.MatchString(lowerName) {
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
