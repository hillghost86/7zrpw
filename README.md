# 7zrpw - 压缩文件密码暴力测试解压工具

![7zrpw](https://github.com/hillghsot86/7zrpw/blob/main/7zrpw.png)
7zrpw 是一个基于 7-Zip 的压缩文件密码破解和解压工具，支持多种压缩格式和中文界面。

## 功能特点

- 通过字典破解压缩文件，并解压文件。

- 支持多种压缩文件格式
  - ZIP (.zip 及其分卷)
  - RAR (.rar 及其分卷)
  - 7Z (.7z)
  - TAR (.tar, .tar.001)
  - GZ (.gz, .tar.gz, .tgz)
  - BZ2 (.bz2, .tar.bz2, .tbz2)
  - XZ (.xz, .tar.xz, .txz)
  - CAB (.cab)
  - ISO (.iso)
  - ARJ (.arj)
  - LZH (.lzh, .lha)
  - WIM (.wim, .swm)

- 智能文件类型检测
  - 通过文件头识别真实文件类型
  - 支持文件扩展名检测
  - 自动识别分卷文件

- 密码破解功能
  - 支持密码字典文件
  - 自动识别是否需要密码
  - 支持手动输入密码

- 便捷操作
  - 支持右键菜单集成
  - 支持拖放文件
  - 自动处理中文编码

## 使用方法

### 基本使用

1. 直接运行程序，按提示选择要处理的文件
2. 或者将文件拖放到程序图标上
3. 或者通过右键菜单选择文件

### 密码字典

密码字典文件名：passwd.txt

程序会按以下顺序查找密码文件 `passwd.txt`：
1. 压缩文件所在当前目录
2. 程序所在目录


密码文件格式：
- 文本文件，每行一个密码
- 支持 UTF-8 和 GBK 编码

### 命令行参数

## 直接命令行使用
'''
bash
7zrpw.exe [选项] [文件路径]
'''

## 安装右键菜单
'''
bash
7zrpw.exe --install
'''

## 卸载右键菜单
'''
bash
7zrpw.exe --uninstall
'''

## 注意事项

- 程序需要管理员权限才能安装/卸载右键菜单
- 部分压缩格式（如 TAR、ISO）本身不支持加密，会直接解压
- 密码破解速度取决于密码复杂度和计算机性能
- 建议使用完整版密码字典以提高破解成功率
- 密码破解结果仅供参考，请自行判断是否正确
## 技术支持
[GitHub Issues](https://github.com/hillghsot86/7zrpw/issues)

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 致谢

- [7-Zip](https://www.7-zip.org/) - 提供核心解压功能
- [h2non/filetype](https://github.com/h2non/filetype) - 提供文件类型检测

