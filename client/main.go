package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 添加调试开关
var debugMode bool = false

// VERSION 程序版本号。由 build.bat 通过 -ldflags "-X main.VERSION=%VER%" 在构建时注入，
// 发版只改 build.bat 的 VER 一处；直接 go build 未注入时为 "dev"（非正式构建标记）。
// 它同时决定界面显示、7z 释放目录(7zrpw_<VERSION>)、自动更新的版本比对，故须保持 vX.Y.Z 格式。
// 注意：-X 只能注入 var 不能注入 const，所以这里必须是 var。
var VERSION = "dev"

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
			// 路径含空格时可能被拆成多个参数，需合并
			filePath := strings.Join(os.Args[1:], " ")
			if _, err := os.Stat(filePath); err != nil {
				// 若合并后仍失败，尝试只用第一个参数（兼容正确传参的情况）
				filePath = os.Args[1]
			}
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
					processArchive(absPath, passwords, passwordsInfo, nil)
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
	runInteractive()
}
