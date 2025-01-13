# 更新日志

所有重要的更新都会记录在这个文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
并且本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [v0.1.1] - 2025-01-12

### 新增
- 支持文件大小显示
- 支持密码字典去重
- 支持解压进度时间显示
- 新作解压用时统计


### 优化
- 优化了文件类型检测逻辑，检测速度更快
- 优化了文件大小显示，更直观
- 优化了密码字典统计，更直观
- 优化了解压错误提示
- 优化了解压路径，解压到压缩包同名的子目录内


### 修复
- 修复了文件类型检测错误的问题





## [v0.1.0] - 2024-03-xx

### 新增
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
  - 优化大文件检测性能

- 密码破解功能
  - 支持密码字典文件
  - 自动识别是否需要密码
  - 支持手动输入密码

- 便捷操作
  - 支持右键菜单集成
  - 支持拖放文件
  - 自动处理中文编码

- 用户界面优化
  - 显示文件类型信息
  - 显示文件大小
  - 显示密码字典统计
  - 优化提示信息

### 修复
- 修复了 RAR 文件解压错误的问题
- 修复了大文件类型检测慢的问题
- 修复了多个密码文件重复的问题

### 优化
- 优化了文件类型检测逻辑
- 改进了错误提示信息
- 优化了内存使用

[v0.1.0]: https://github.com/hillghost86/7zrpw/releases/tag/v0.1.0