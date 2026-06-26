package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// contextMenuFail 统一打印右键菜单操作失败的提示并返回错误
// action: "安装" 或 "卸载"；reason: 失败原因；adminHint: 是否追加「请以管理员身份运行」提示
func contextMenuFail(action, reason string, err error, adminHint bool) error {
	fmt.Println("----------------------------------")
	fmt.Printf("  xxxxxx %s失败 xxxxxx\n", action)
	fmt.Println("(╯°□°）╯︵ ┻━┻")
	if adminHint {
		fmt.Printf("%s: %v\n请以【管理员身份运行】后重试。\n", reason, err)
	} else {
		fmt.Printf("%s: %v\n", reason, err)
	}
	fmt.Printf("右键菜单%s失败！\n", action)
	fmt.Println("----------------------------------")
	if adminHint {
		return fmt.Errorf("%s: %v \n请以【管理员身份运行】后重试。", reason, err)
	}
	return fmt.Errorf("%s: %v", reason, err)
}

// contextMenuOK 统一打印右键菜单操作成功的提示
func contextMenuOK(action string) {
	fmt.Println("----------------------------------")
	fmt.Printf("  ✓✓✓✓✓ %s成功 ✓✓✓✓✓\n", action)
	fmt.Println("	( •̀ ω •́ )✧")
	fmt.Printf("右键菜单%s成功！\n", action)
	fmt.Println("----------------------------------")
}

// 函数说明：安装右键菜单
// 返回：错误信息
func installContext() error {
	// 获取程序路径
	exe, err := os.Executable()
	if err != nil {
		return contextMenuFail("安装", "获取程序路径失败", err, false)
	}
	exePath := strings.ReplaceAll(exe, "/", "\\")

	// 创建注册表项
	k, _, err := registry.CreateKey(
		registry.CLASSES_ROOT,
		`*\shell\7zrpw`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return contextMenuFail("安装", "创建注册表项失败", err, true)
	}
	defer k.Close()

	// 设置显示名称和图标
	if err := k.SetStringValue("", "使用7zrpw解压"); err != nil {
		return contextMenuFail("安装", "设置显示名称失败", err, true)
	}
	if err := k.SetStringValue("Icon", exePath); err != nil {
		return contextMenuFail("安装", "设置图标失败", err, true)
	}

	// 创建command子项
	k2, _, err := registry.CreateKey(
		registry.CLASSES_ROOT,
		`*\shell\7zrpw\command`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return contextMenuFail("安装", "创建command子项失败", err, true)
	}
	defer k2.Close()

	// 设置命令
	command := fmt.Sprintf("\"%s\" \"%%1\"", exePath)
	if err := k2.SetStringValue("", command); err != nil {
		return contextMenuFail("安装", "设置命令失败", err, true)
	}

	// 安装成功
	contextMenuOK("安装")
	return nil
}

// 函数说明：卸载右键菜单
// 返回：错误信息
func uninstallContext() error {
	// 删除注册表项
	if err := registry.DeleteKey(registry.CLASSES_ROOT, `*\shell\7zrpw\command`); err != nil {
		return contextMenuFail("卸载", "删除command子项失败", err, true)
	}

	if err := registry.DeleteKey(registry.CLASSES_ROOT, `*\shell\7zrpw`); err != nil {
		return contextMenuFail("卸载", "删除注册表项失败", err, true)
	}

	// 卸载成功
	contextMenuOK("卸载")
	return nil
}
