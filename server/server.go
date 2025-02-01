package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// 版本信息结构
type VersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	ReleaseNote string `json:"release_note"`
	MD5         string `json:"md5"`
	ForceUpdate bool   `json:"force_update"`
}

// 服务器配置结构
type ServerConfig struct {
	Port        int         `json:"port"`
	UpdateInfo  VersionInfo `json:"update_info"`
	DownloadDir string      `json:"download_dir"`
}

var (
	config    ServerConfig
	serverDir string        // 服务器目录
	debugMode bool   = true // 添加调试模式变量，默认开启
)

func init() {
	// 获取程序所在目录
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("获取程序路径失败:", err)
	}
	execDir := filepath.Dir(exePath)

	// 检查是否在开发环境中运行
	if strings.Contains(execDir, "go-build") {
		// 在开发环境中，使用当前工作目录
		workDir, err := os.Getwd()
		if err != nil {
			log.Fatal("获取工作目录失败:", err)
		}
		serverDir = filepath.Join(workDir, "server")
	} else {
		// 在生产环境中，使用可执行文件所在目录
		serverDir = execDir
	}
}

func main() {
	// 加载配置文件
	if err := loadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 确保下载目录存在（使用绝对路径）
	downloadDir := filepath.Join(serverDir, config.DownloadDir)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Fatal("创建下载目录失败:", err)
	}

	// 设置路由
	http.HandleFunc("/api/v1/version", handleVersion)
	http.HandleFunc("/downloads/", handleDownload)
	http.HandleFunc("/api/v1/reload", handleReload)

	fmt.Printf("服务器目录: %s\n", serverDir)
	fmt.Printf("下载目录: %s\n", downloadDir)
	fmt.Printf("服务器启动在 http://localhost:%d\n", config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil))
}

// 加载配置文件
func loadConfig() error {
	configPath := filepath.Join(serverDir, "config.json")

	// 默认配置
	config = ServerConfig{
		Port:        8080,
		DownloadDir: "downloads", // 相对路径
		UpdateInfo: VersionInfo{
			Version:     "v0.1.0",
			DownloadURL: "http://localhost:8080/downloads/7zrpw.exe",
			ReleaseNote: "N/A",
			ForceUpdate: false,
		},
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果配置文件不存在，创建默认配置
			return saveConfig(configPath)
		}
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析配置
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	return nil
}

// 保存配置文件
func saveConfig(configPath string) error {
	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %v", err)
	}

	return nil
}

// 处理版本检查请求
func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	// 每次请求前重新读取配置文件
	if err := loadConfig(); err != nil {
		log.Printf("重新加载配置失败: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "加载配置失败",
		})
		return
	}

	// 使用绝对路径
	updateFile := filepath.Join(serverDir, config.DownloadDir, filepath.Base(config.UpdateInfo.DownloadURL))

	// 检查更新文件是否存在
	if _, err := os.Stat(updateFile); os.IsNotExist(err) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "更新文件不存在",
		})
		return
	}

	// 计算MD5
	md5hash, err := calculateMD5(updateFile)
	if err != nil {
		log.Printf("计算MD5失败: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// 创建响应对象
	response := VersionInfo{
		Version:     config.UpdateInfo.Version,
		DownloadURL: config.UpdateInfo.DownloadURL,
		ReleaseNote: config.UpdateInfo.ReleaseNote,
		MD5:         md5hash,
		ForceUpdate: config.UpdateInfo.ForceUpdate,
	}

	if debugMode {
		log.Printf("配置文件中的强制更新标志: %v\n", config.UpdateInfo.ForceUpdate)
		log.Printf("响应中的强制更新标志: %v\n", response.ForceUpdate)
		responseJSON, _ := json.MarshalIndent(response, "", "    ")
		log.Printf("完整响应内容:\n%s\n", string(responseJSON))
	}

	// 返回JSON响应
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("编码JSON失败: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// 处理文件下载请求
func handleDownload(w http.ResponseWriter, r *http.Request) {
	// 每次请求前重新读取配置文件
	if err := loadConfig(); err != nil {
		log.Printf("重新加载配置失败: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	filename := filepath.Base(r.URL.Path)
	filepath := filepath.Join(serverDir, config.DownloadDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Type", "application/octet-stream")

	// 发送文件
	http.ServeFile(w, r, filepath)
}

// 计算文件MD5
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

// 处理重新加载配置请求
func handleReload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if err := loadConfig(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("重新加载配置失败: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"message": "配置已重新加载",
	})
}
