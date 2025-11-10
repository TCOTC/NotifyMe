package tray

import (
	_ "embed"
	"fmt"
	"notifyme/internal/logger"
	"os"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var iconData []byte

// GetIconData 获取图标数据（供其他包使用）
func GetIconData() []byte {
	return iconData
}

var (
	onOpenUI   func()
	onQuit     func()
	menuOpenUI *systray.MenuItem
	menuQuit   *systray.MenuItem
)

// Init 初始化系统托盘
func Init(onOpenUICallback, onQuitCallback func()) {
	onOpenUI = onOpenUICallback
	onQuit = onQuitCallback

	go func() {
		systray.Run(onReady, onExit)
	}()
}

// onReady 托盘就绪回调
func onReady() {
	// 设置图标和标题
	systray.SetIcon(getIcon())
	// 在工具提示中显示进程 ID，方便用户查找进程
	pid := os.Getpid()
	systray.SetTooltip(fmt.Sprintf("NotifyMe - 消息通知 (PID: %d)", pid))

	// 添加菜单项
	menuOpenUI = systray.AddMenuItem("打开界面", "打开主界面")
	systray.AddSeparator()
	menuQuit = systray.AddMenuItem("退出", "退出程序")

	// 监听菜单点击事件
	// 注意：这个 goroutine 会一直运行，直到 systray.Run() 退出
	go func() {
		for {
			select {
			case <-menuOpenUI.ClickedCh:
				logger.Info("点击打开界面菜单项")
				if onOpenUI != nil {
					// 在 goroutine 中调用，避免阻塞事件循环
					go func() {
						defer func() {
							if r := recover(); r != nil {
								logger.Errorf("打开界面时发生错误: %v", r)
							}
						}()
						onOpenUI()
					}()
				} else {
					logger.Warn("打开界面回调未设置")
				}
			case <-menuQuit.ClickedCh:
				logger.Info("点击退出菜单项")
				if onQuit != nil {
					// 在 goroutine 中调用，避免阻塞事件循环
					go func() {
						defer func() {
							if r := recover(); r != nil {
								logger.Errorf("退出程序时发生错误: %v", r)
								// 如果退出失败，直接退出托盘
								systray.Quit()
							}
						}()
						onQuit()
						// onQuit 会调用 app.Quit()，app.Quit() 内部会调用 tray.Quit()
						// 所以这里不需要再次调用 systray.Quit()
					}()
				} else {
					logger.Warn("退出回调未设置，直接退出托盘")
					// 如果没有设置退出回调，直接退出托盘
					systray.Quit()
				}
			}
		}
	}()
}

// onExit 托盘退出回调
func onExit() {
	logger.Info("系统托盘退出")
}

// getIcon 获取托盘图标
func getIcon() []byte {
	// 如果嵌入的图标数据为空，返回 nil（systray 会使用默认图标）
	if len(iconData) == 0 {
		return nil
	}
	return iconData
}

// Quit 退出托盘
func Quit() {
	systray.Quit()
}

// SetTooltip 设置托盘提示
func SetTooltip(tooltip string) {
	systray.SetTooltip(tooltip)
}
