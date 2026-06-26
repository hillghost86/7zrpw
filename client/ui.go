package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

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

// runInteractive 交互模式主循环：目录浏览、选文件破解、安装/卸载右键菜单等
func runInteractive() {
	reader := bufio.NewReader(os.Stdin)
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
				// 普通输入（使用 readLineInput 支持路径中含空格，如拖入文件）
				choice = readLineInput(reader)
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
					processArchive(file, passwords, passwordsInfo, reader)
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
			choice = readLineInput(reader)

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
			dictPath = readLineInput(reader)
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

		processArchive(archivePath, passwords, passwordsInfo, reader)
		// 处理更新和退出
		handleUpdateAndExit()

		fmt.Print("\n按回车键继续...")
		fmt.Scanln()
	}
}
