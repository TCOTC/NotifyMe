# NotifyMe

一个基于 Wails 框架开发的跨平台消息通知管理应用，支持监控 GitHub 和 LD246 网站的状态变化，并通过 Windows 系统通知及时提醒用户。

## ✨ 主要功能

- 🔔 **多源监控**：支持监控 GitHub 和 LD246 网站的状态变化
- 📢 **系统通知**：通过 Windows 原生通知系统及时提醒用户
- 🎯 **系统托盘**：最小化到系统托盘，不占用任务栏空间
- 🔒 **单实例运行**：确保应用程序只运行一个实例，避免重复启动
- ⏰ **定时轮询**：可配置的轮询间隔，自动检查状态变化
- ⚙️ **配置管理**：支持保存和加载应用配置
- 📝 **日志记录**：完整的日志系统，便于问题排查

## 🛠️ 技术栈

- **后端**：Go 1.25.0+
- **前端**：Vite + pnpm
- **框架**：Wails v2.11.0+
- **平台**：Windows 10+（主要），macOS（支持）

## 📋 前置要求

- **Go 1.25.0+**：[下载地址](https://golang.org/dl/)
- **Node.js 16.x+** 和 **pnpm**：[下载地址](https://nodejs.org/)，安装 pnpm：`npm install -g pnpm`
- **Wails CLI v2.11.0+**：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Windows 10+** 和 **WebView2 运行时**（通常已预装）

验证安装：
```bash
go version
node --version
pnpm --version
wails version
```

## 🚀 快速开始

### 使用构建脚本（推荐）

```powershell
# 标准构建
.\build.ps1

# 可用选项：-Clean（清理）、-Debug（调试模式）、-Compress（压缩）、-SkipFrontend（跳过前端）
.\build.ps1 -Clean -Compress
```

### 使用 Wails 命令

```bash
# 标准构建
wails build

# 常用选项
wails build -dev          # 开发模式
wails build -clean        # 清理构建目录
wails build -compress     # 压缩输出
wails build -s            # 跳过前端构建
```

构建输出：`build/bin/NotifyMe.exe`

### 开发模式

```bash
# 启动开发服务器（前端热重载）
wails dev
```

## 📁 项目结构

```
NotifyMe/
├── build/                 # 构建相关目录
│   ├── bin/              # 编译后的可执行文件
│   ├── windows/          # Windows 平台构建配置
│   └── darwin/           # macOS 平台构建配置
├── frontend/             # 前端代码目录
│   ├── src/              # 前端源代码
│   ├── dist/             # 前端构建产物
│   └── package.json      # 前端依赖配置
├── internal/             # 内部 Go 包（不对外暴露）
│   ├── auth/             # 认证模块（GitHub、LD246）
│   ├── config/           # 配置管理模块
│   ├── logger/           # 日志模块
│   ├── monitor/          # 监控模块（GitHub、LD246）
│   ├── notifier/         # 通知模块（Windows）
│   ├── scheduler/        # 任务调度器
│   ├── singleinstance/   # 单实例控制
│   └── tray/             # 系统托盘模块
├── pkg/                  # 公共 Go 包（可对外暴露）
│   └── types/            # 类型定义
├── app.go                # 应用主逻辑
├── main.go               # 程序入口
├── build.ps1             # PowerShell 构建脚本
└── wails.json            # Wails 框架配置
```

## 🔧 主要模块说明

### 核心功能模块（internal/）

- **auth/**: 处理 GitHub 和 LD246 网站的认证逻辑
- **config/**: 管理应用程序配置的加载和保存
- **logger/**: 提供统一的日志记录功能
- **monitor/**: 监控 GitHub 和 LD246 网站的状态变化
- **notifier/**: 实现 Windows 平台的通知功能
- **scheduler/**: 任务调度器，管理定时任务和轮询
- **singleinstance/**: 确保应用程序只运行一个实例
- **tray/**: 系统托盘图标和菜单功能

### 前端模块（frontend/）

- 使用 Vite 作为构建工具
- 使用 pnpm 作为包管理器
- 通过 Wails 框架与 Go 后端通信

## 📖 使用说明

1. **首次运行**：启动应用后，程序会最小化到系统托盘
2. **打开窗口**：右键点击系统托盘图标，选择"打开"或双击托盘图标
3. **配置设置**：在主窗口中配置监控源和轮询间隔
4. **查看通知**：当检测到状态变化时，会弹出 Windows 系统通知
5. **退出程序**：右键点击系统托盘图标，选择"退出"

## ❓ 常见问题

**找不到 wails 命令**：确保 `$GOPATH/bin` 或 `$GOBIN` 在 PATH 中

**前端构建失败**：删除 `frontend/node_modules` 和 `frontend/pnpm-lock.yaml`，重新执行 `cd frontend && pnpm install`

**Go 模块下载失败**：配置代理 `go env -w GOPROXY=https://goproxy.cn,direct` 或检查网络连接

**WebView2 错误**：从 [Microsoft Edge WebView2](https://developer.microsoft.com/microsoft-edge/webview2/) 下载安装

## 📝 开发说明

项目结构说明请参考 [项目结构说明.md](./项目结构说明.md)

## 📄 许可证

本项目采用 MIT 许可证。

## 👤 作者

**Jeffrey Chen**

- GitHub: [@TCOTC](https://github.com/TCOTC)
- Email: 78434827+TCOTC@users.noreply.github.com

## 🙏 致谢

- [Wails](https://wails.io/) - 用于构建跨平台桌面应用
- [Vite](https://vitejs.dev/) - 前端构建工具

