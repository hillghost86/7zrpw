package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// 版本信息结构
type VersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	ReleaseNote string `json:"release_note"`
	MD5         string `json:"md5"`
	ForceUpdate bool   `json:"force_update"` // 确保字段名完全匹配
	SHA256      string `json:"sha256"`       // 新机制：校验和（验签用），老客户端忽略此字段
	Signature   string `json:"signature"`    // 新机制：Ed25519 签名(base64)，老客户端忽略此字段
}

//go:embed update_pub.key
var updatePubKeyB64 string

// updatePublicKey 内置的更新签名公钥（Ed25519），用于验证下载的新版本是否由官方私钥签名。
// 公钥可公开；私钥离线保管（见 tools/sign 与方案文档）。缺失公钥文件会导致编译失败（有意为之）。
var updatePublicKey ed25519.PublicKey

func init() {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(updatePubKeyB64))
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("内置更新公钥无效（client/update_pub.key）")
	}
	updatePublicKey = ed25519.PublicKey(b)
}

// UpdateManager 更新管理器
type UpdateManager struct {
	CurrentVersion string
}

// 函数说明：异步检查更新
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

// handleUpdateAndExit 处理更新检查和程序退出
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

// NewUpdateManager 创建更新管理器
func NewUpdateManager(version string) (*UpdateManager, error) {
	manager := &UpdateManager{
		CurrentVersion: version,
	}
	return manager, nil
}

// 添加全局变量存储更新信息
var (
	updateResultChan = make(chan string, 1)
	updateManager    *UpdateManager
	updateInfo       VersionInfo
)

// CheckUpdate 检查更新
func (m *UpdateManager) CheckUpdate(force bool) error {
	fmt.Printf("当前版本: %s\n", m.CurrentVersion)

	// 从服务器获取版本信息（带超时，避免网络异常时长时间阻塞）
	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Get("https://down.pp.ci/api/v1/version")
	if err != nil {
		return fmt.Errorf("无法连接到更新服务器，请稍后重试")
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新服务器暂时不可用 (HTTP %d)", resp.StatusCode)
	}

	// 读取和解析版本信息
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取服务器响应失败")
	}

	var info VersionInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("解析版本信息失败")
	}

	if info.Version == "" || info.DownloadURL == "" {
		return fmt.Errorf("服务器返回的版本信息不完整")
	}

	//使用compareVersions函数判断版本是否需要更新
	compareResult := m.compareVersions(info.Version)
	hasNewVersion := compareResult == 1
	needUpdate := hasNewVersion || force

	// 如果版本相同且是服务器强制更新，则忽略
	if !hasNewVersion && info.ForceUpdate && !force {
		return nil
	}

	if !needUpdate {
		// 将"当前已是最新版本"消息通过通道传递
		select {
		case updateResultChan <- "当前已是最新版本":
		default:
		}
		return nil
	}

	if hasNewVersion {
		// 保存更新信息到全局变量
		updateInfo = info
		updateManager = m // 确保 updateManager 被正确设置

		// 构建更新消息
		var updateMsg strings.Builder
		updateMsg.WriteString(fmt.Sprintf("\n发现新版本: %s\n", info.Version))
		if info.MD5 != "" {
			updateMsg.WriteString(fmt.Sprintf("新版本MD5: %s\n", info.MD5))
		}
		if info.ReleaseNote != "" {
			updateMsg.WriteString(fmt.Sprintf("更新说明:\n%s\n", info.ReleaseNote))
		}
		updateMsg.WriteString(fmt.Sprintf("手动下载地址: %s", info.DownloadURL))

		// 将消息发送到通道
		updateResultChan <- updateMsg.String()
	}

	// 执行更新
	if info.ForceUpdate && hasNewVersion {
		fmt.Println("\n这是一个强制更新，系统将自动执行更新...")
		return m.doUpdate(info)
	}

	return nil
}

// doUpdate 下载、校验、验签并原地替换为新版本
// 取代旧的 update.bat 自替换方案：新增 sha256 完整性校验 + Ed25519 签名验签，
// 由 minio/selfupdate 负责原地替换正在运行的 exe（Windows 改名）与失败回滚。
func (m *UpdateManager) doUpdate(info VersionInfo) error {
	fmt.Println("\n=== 开始更新过程 ===")

	// 合规要点（fail-closed）：新客户端只接受带 sha256 + 签名的更新，
	// 缺任一项即拒绝，防止「降级/签名剥离」攻击（绝不回退到无签名的旧路径）
	if info.SHA256 == "" || info.Signature == "" {
		return fmt.Errorf("服务端未提供校验和或签名，出于安全已拒绝更新")
	}
	wantSum, err := hex.DecodeString(strings.TrimSpace(info.SHA256))
	if err != nil || len(wantSum) != sha256.Size {
		return fmt.Errorf("服务端 sha256 字段无效")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(info.Signature))
	if err != nil {
		return fmt.Errorf("服务端 signature 字段无效")
	}

	// 下载新版本（带总超时）
	fmt.Printf("正在下载新版本: %s\n", info.DownloadURL)
	httpClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := httpClient.Get(info.DownloadURL)
	if err != nil {
		return fmt.Errorf("下载失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败 (HTTP %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取下载内容失败: %v", err)
	}
	fmt.Printf("下载完成，大小 %.2f MB\n", float64(len(data))/1024/1024)

	// 校验和（完整性）
	sum := sha256.Sum256(data)
	if !bytes.Equal(sum[:], wantSum) {
		return fmt.Errorf("校验和不匹配，文件可能损坏或被篡改，已拒绝更新")
	}
	// 验签（真伪）：用内置公钥验证「对 sha256 摘要的 Ed25519 签名」
	if !ed25519.Verify(updatePublicKey, sum[:], sig) {
		return fmt.Errorf("签名验证失败，更新包不可信，已拒绝更新")
	}
	fmt.Println("校验和与签名验证通过")
	fmt.Printf("即将从 %s 更新到 %s ...\n", m.CurrentVersion, info.Version)

	// 原地替换正在运行的 exe：selfupdate 负责 Windows 下的替换与失败自动回滚
	if err := selfupdate.Apply(bytes.NewReader(data), selfupdate.Options{Checksum: sum[:]}); err != nil {
		// 兜底：替换中途失败时尝试回滚到旧版本
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("更新失败且回滚失败（程序可能已损坏，请手动重装）: %v / 回滚错误: %v", err, rerr)
		}
		return fmt.Errorf("更新失败，已回滚到旧版本: %v", err)
	}

	fmt.Println("\n✓ 更新成功！正在重启新版本...")
	return restartSelf()
}

// restartSelf 启动替换后的新 exe 并退出当前进程（更新后自动重启）
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("更新已完成，但获取程序路径失败，请手动重新打开程序。")
		os.Exit(0)
	}
	cmd := exec.Command(exe)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		// 边界：自动重启失败不影响「已更新成功」这个事实，提示用户手动打开
		fmt.Printf("更新已完成，但自动重启失败，请手动重新打开程序。(%v)\n", err)
	}
	os.Exit(0)
	return nil
}

// 版本比较函数
// 返回值：
// 0: 版本相同
// 1: 有新版本
// -1: 当前版本更新
func (um *UpdateManager) compareVersions(newVersion string) int {
	// 移除版本号前缀的'v'
	current := strings.TrimPrefix(um.CurrentVersion, "v")
	new := strings.TrimPrefix(newVersion, "v")

	// 分割版本号
	currentParts := strings.Split(current, ".")
	newParts := strings.Split(new, ".")

	// 按数字比较每个部分（如 "2" < "10"）
	for i := 0; i < len(currentParts) && i < len(newParts); i++ {
		curN, curOk := parseVersionPart(currentParts[i])
		newN, newOk := parseVersionPart(newParts[i])

		if curOk && newOk {
			// 均为数字，按数值比较
			if curN < newN {
				return 1
			}
			if curN > newN {
				return -1
			}
		} else {
			// 含非数字，回退到字符串比较
			if currentParts[i] < newParts[i] {
				return 1
			}
			if currentParts[i] > newParts[i] {
				return -1
			}
		}
	}

	// 前面都相同，比较版本号长度
	if len(newParts) > len(currentParts) {
		return 1
	}
	if len(newParts) < len(currentParts) {
		return -1
	}

	return 0 // 版本相同
}

// parseVersionPart 解析版本段为数字，支持 "1"、"10"、"0" 等
func parseVersionPart(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return n, true
}
