package notifier

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"notifyme/internal/logger"
	"notifyme/internal/tray"
	"notifyme/pkg/types"

	"github.com/go-toast/toast"
)

// WindowsNotifier Windows 系统通知器
type WindowsNotifier struct {
	notifiedIDs map[string]bool
	mu          sync.RWMutex
	iconPath    string // 图标文件路径
}

// NewWindowsNotifier 创建新的 Windows 通知器
func NewWindowsNotifier() *WindowsNotifier {
	notifier := &WindowsNotifier{
		notifiedIDs: make(map[string]bool),
	}

	// 初始化图标路径
	notifier.initIcon()

	return notifier
}

// initIcon 初始化通知图标
func (n *WindowsNotifier) initIcon() {
	// 从 tray 包获取图标数据
	iconData := tray.GetIconData()

	// 如果图标数据为空，跳过
	if len(iconData) == 0 {
		logger.Warn("通知图标数据为空，将使用默认图标")
		return
	}

	// 获取临时目录
	tempDir := os.TempDir()
	iconPath := filepath.Join(tempDir, "notifyme_icon.ico")

	// 如果图标文件已存在，直接使用
	if _, err := os.Stat(iconPath); err == nil {
		n.iconPath = iconPath
		return
	}

	// 写入图标文件
	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		logger.Warnf("写入通知图标文件失败: %v，将使用默认图标", err)
		return
	}

	n.iconPath = iconPath
	logger.Debugf("通知图标已初始化: %s", iconPath)
}

// Notify 发送通知
func (n *WindowsNotifier) Notify(notification *types.Notification) error {
	// 检查是否已经通知过
	n.mu.RLock()
	if n.notifiedIDs[notification.ID] {
		n.mu.RUnlock()
		logger.Debugf("通知已发送过，跳过: %s", notification.ID)
		return nil
	}
	n.mu.RUnlock()

	// 构建通知内容
	title := notification.Title
	message := notification.Content
	if message == "" {
		message = "点击查看详情"
	}

	// 创建通知
	// 尝试使用图标文件路径作为 AppID，这样 Windows 可以直接从图标文件显示图标
	appID := "NotifyMe"
	if n.iconPath != "" {
		// 使用图标文件路径作为 AppID，Windows 会从图标文件中提取图标显示在标题区域
		absIconPath, err := filepath.Abs(n.iconPath)
		if err == nil {
			appID = absIconPath
		}
	}

	notificationToast := toast.Notification{
		AppID:   appID,
		Title:   title,
		Message: message,
		Actions: []toast.Action{
			{
				Type:      "protocol",
				Label:     "打开",
				Arguments: notification.Link,
			},
		},
		ActivationType: "protocol",
	}
	// 注意：不设置 Icon 字段，避免在内容区域显示图标

	// 发送通知
	if err := notificationToast.Push(); err != nil {
		return fmt.Errorf("发送通知失败: %w", err)
	}

	// 标记为已通知
	n.mu.Lock()
	n.notifiedIDs[notification.ID] = true
	n.mu.Unlock()

	logger.Infof("已发送通知: %s - %s", notification.Title, notification.ID)
	return nil
}

// NotifyBatch 批量发送通知
func (n *WindowsNotifier) NotifyBatch(notifications []*types.Notification) error {
	for _, notification := range notifications {
		if err := n.Notify(notification); err != nil {
			logger.Errorf("发送通知失败: %v", err)
			// 继续发送其他通知，不中断
		}
	}
	return nil
}

// IsNotified 检查是否已通知
func (n *WindowsNotifier) IsNotified(id string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.notifiedIDs[id]
}

// ClearNotified 清空已通知记录（程序重启时调用）
func (n *WindowsNotifier) ClearNotified() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifiedIDs = make(map[string]bool)
	logger.Info("已清空通知记录")
}
