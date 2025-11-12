package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"notifyme/internal/logger"
	"notifyme/internal/monitor"
	"notifyme/internal/notifier"
	"notifyme/pkg/types"
)

// Scheduler 轮询调度器
type Scheduler struct {
	ld246Monitor  *monitor.Ld246Monitor
	githubMonitor *monitor.GitHubMonitor
	notifier      *notifier.WindowsNotifier
	config        *types.Config
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	running       bool
	mu            sync.RWMutex
	// 最近的通知列表（最多 50 条）
	recentNotifications []*types.Notification
	notificationsMu     sync.RWMutex
}

// NewScheduler 创建新的调度器
func NewScheduler(cfg *types.Config) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		ld246Monitor:  monitor.NewLd246Monitor(cfg.Ld246.Token),
		githubMonitor: monitor.NewGitHubMonitor(cfg.GitHub.Token),
		notifier:      notifier.NewWindowsNotifier(),
		config:        cfg,
		ctx:           ctx,
		cancel:        cancel,
		running:       false,
	}

	// 加载保存的通知列表
	if err := s.loadNotifications(); err != nil {
		logger.Warnf("加载通知列表失败: %v", err)
	}

	return s
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	logger.Info("启动轮询调度器")

	// 启动 ld246 监控
	s.wg.Add(1)
	go s.runLd246Monitor()

	// 启动 GitHub 监控
	s.wg.Add(1)
	go s.runGitHubMonitor()

	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	logger.Info("停止轮询调度器")
	s.cancel()

	// 使用带超时的等待，避免因为网络请求阻塞而无法退出
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("调度器已完全停止")
	case <-time.After(3 * time.Second):
		logger.Warn("等待调度器停止超时，强制继续退出")
	}
}

// IsRunning 检查是否正在运行
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// UpdateConfig 更新配置
func (s *Scheduler) UpdateConfig(cfg *types.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
	s.ld246Monitor = monitor.NewLd246Monitor(cfg.Ld246.Token)
	s.githubMonitor = monitor.NewGitHubMonitor(cfg.GitHub.Token)
}

// runLd246Monitor 运行 ld246 监控
func (s *Scheduler) runLd246Monitor() {
	defer s.wg.Done()

	interval := time.Duration(s.config.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即执行一次
	s.checkLd246()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkLd246()
		}
	}
}

// runGitHubMonitor 运行 GitHub 监控
func (s *Scheduler) runGitHubMonitor() {
	defer s.wg.Done()

	interval := time.Duration(s.config.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即执行一次
	s.checkGitHub()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkGitHub()
		}
	}
}

// checkLd246 检查 ld246 新消息
func (s *Scheduler) checkLd246() {
	logger.Debug("检查 ld246 新消息...")

	// 获取最近回帖
	replies, err := s.ld246Monitor.FetchRecentReplies()
	if err != nil {
		logger.Errorf("获取 ld246 最近回帖失败: %v", err)
	} else {
		if len(replies) > 0 {
			logger.Infof("ld246: 获取到 %d 条最近回帖，准备发送和添加到列表", len(replies))
			s.notifier.NotifyBatch(replies)
			s.addNotifications(replies)
		}
	}

	// 获取未读消息
	messages, err := s.ld246Monitor.FetchUnreadMessages()
	if err != nil {
		logger.Errorf("获取 ld246 未读消息失败: %v", err)
	} else {
		if len(messages) > 0 {
			logger.Infof("ld246: 获取到 %d 条未读消息，准备发送和添加到列表", len(messages))
			s.notifier.NotifyBatch(messages)
			s.addNotifications(messages)
		}
	}

	logger.Info("ld246 检查完成")
}

// checkGitHub 检查 GitHub 新通知
func (s *Scheduler) checkGitHub() {
	logger.Debug("检查 GitHub 新通知...")

	notifications, err := s.githubMonitor.FetchNotifications()
	if err != nil {
		logger.Errorf("获取 GitHub 通知失败: %v", err)
		logger.Info("GitHub 检查完成")
		return
	}

	if len(notifications) > 0 {
		logger.Infof("GitHub: 获取到 %d 条通知，准备发送和添加到列表", len(notifications))
		s.notifier.NotifyBatch(notifications)
		s.addNotifications(notifications)
	}

	logger.Info("GitHub 检查完成")
}

// addNotifications 添加通知到最近通知列表（插入到顶部，最多保留 50 条）
// 如果通知已存在，会将其移动到列表最前面
func (s *Scheduler) addNotifications(notifications []*types.Notification) {
	if len(notifications) == 0 {
		return
	}

	s.notificationsMu.Lock()

	// 创建已存在通知 ID 的集合，用于去重和查找
	existingIDs := make(map[string]bool)
	for _, notif := range s.recentNotifications {
		existingIDs[notif.ID] = true
	}

	// 分离新通知和已存在的通知
	newNotifications := make([]*types.Notification, 0, len(notifications))
	existingNotifications := make([]*types.Notification, 0, len(notifications))
	seenIDs := make(map[string]bool) // 用于同一批次内去重

	for _, notif := range notifications {
		// 同一批次内去重
		if seenIDs[notif.ID] {
			continue
		}
		seenIDs[notif.ID] = true

		if existingIDs[notif.ID] {
			// 已存在的通知，需要移动到最前面
			existingNotifications = append(existingNotifications, notif)
		} else {
			// 新通知
			newNotifications = append(newNotifications, notif)
		}
	}

	// 如果既没有新通知也没有需要移动的通知，直接返回
	if len(newNotifications) == 0 && len(existingNotifications) == 0 {
		s.notificationsMu.Unlock()
		logger.Debug("所有通知都已存在于列表中，跳过添加")
		return
	}

	// 移除已存在的通知（从原位置删除）
	if len(existingNotifications) > 0 {
		existingIDsToRemove := make(map[string]bool)
		for _, notif := range existingNotifications {
			existingIDsToRemove[notif.ID] = true
		}

		// 过滤掉需要移动的通知
		filteredNotifications := make([]*types.Notification, 0, len(s.recentNotifications))
		for _, notif := range s.recentNotifications {
			if !existingIDsToRemove[notif.ID] {
				filteredNotifications = append(filteredNotifications, notif)
			}
		}
		s.recentNotifications = filteredNotifications

		logger.Debugf("从列表中移除 %d 条已存在的通知，准备移动到最前面", len(existingNotifications))
	}

	// 将需要移动的通知和新通知都插入到列表顶部（先插入需要移动的，再插入新的）
	notificationsToAdd := append(existingNotifications, newNotifications...)
	s.recentNotifications = append(notificationsToAdd, s.recentNotifications...)

	// 如果超过 50 条，删除末尾的
	if len(s.recentNotifications) > 50 {
		s.recentNotifications = s.recentNotifications[:50]
	}

	logger.Infof("添加 %d 条新通知到列表，移动 %d 条已存在的通知到最前面（共处理 %d 条通知）",
		len(newNotifications), len(existingNotifications), len(notifications))

	// 复制数据用于保存（在释放锁之前）
	notificationsToSave := make([]*types.Notification, len(s.recentNotifications))
	copy(notificationsToSave, s.recentNotifications)

	s.notificationsMu.Unlock()

	// 保存到文件（在锁外执行，避免死锁）
	if err := s.saveNotificationsWithData(notificationsToSave); err != nil {
		logger.Warnf("保存通知列表失败: %v", err)
	} else {
		logger.Infof("成功保存 %d 条通知到文件", len(notificationsToSave))
	}
}

// GetRecentNotifications 获取最近的通知列表
func (s *Scheduler) GetRecentNotifications() []*types.Notification {
	s.notificationsMu.RLock()
	defer s.notificationsMu.RUnlock()

	// 返回副本，避免外部修改
	result := make([]*types.Notification, len(s.recentNotifications))
	copy(result, s.recentNotifications)
	return result
}

// TriggerCheck 手动触发检查（立即检查所有监控源）
func (s *Scheduler) TriggerCheck() {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	if !running {
		logger.Warn("调度器未运行，无法触发检查")
		return
	}

	logger.Info("手动触发检查...")

	// 在 goroutine 中执行检查，避免阻塞
	go func() {
		s.checkLd246()
		s.checkGitHub()
	}()
}

// getNotificationsFilePath 获取通知列表文件路径
func (s *Scheduler) getNotificationsFilePath() string {
	// 优先使用当前目录（与配置文件逻辑保持一致）
	dataDir := filepath.Join(".", "data")
	if _, err := os.Stat(dataDir); err == nil {
		return filepath.Join(dataDir, "notifications.json")
	}

	// 如果当前目录不存在，使用用户配置目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dataDir := filepath.Join(homeDir, ".notifyme", "data")
		os.MkdirAll(dataDir, 0755)
		return filepath.Join(dataDir, "notifications.json")
	}

	// 如果无法获取用户目录，使用当前目录（即使不存在也会在保存时创建）
	return filepath.Join(dataDir, "notifications.json")
}

// saveNotifications 保存通知列表到文件（从当前列表读取）
func (s *Scheduler) saveNotifications() error {
	s.notificationsMu.RLock()
	notifications := make([]*types.Notification, len(s.recentNotifications))
	copy(notifications, s.recentNotifications)
	s.notificationsMu.RUnlock()

	return s.saveNotificationsWithData(notifications)
}

// saveNotificationsWithData 保存指定的通知列表到文件
func (s *Scheduler) saveNotificationsWithData(notifications []*types.Notification) error {
	filePath := s.getNotificationsFilePath()
	dataDir := filepath.Dir(filePath)

	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(notifications, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化通知列表失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	logger.Infof("通知列表已保存到文件: %s (共 %d 条)", filePath, len(notifications))
	return nil
}

// loadNotifications 从文件加载通知列表
func (s *Scheduler) loadNotifications() error {
	filePath := s.getNotificationsFilePath()

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Debug("通知列表文件不存在，跳过加载")
		return nil
	}

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 反序列化
	var notifications []*types.Notification
	if err := json.Unmarshal(data, &notifications); err != nil {
		return fmt.Errorf("解析通知列表失败: %w", err)
	}

	// 限制最多 50 条
	if len(notifications) > 50 {
		notifications = notifications[:50]
	}

	// 更新通知列表
	s.notificationsMu.Lock()
	s.recentNotifications = notifications
	s.notificationsMu.Unlock()

	logger.Infof("成功加载 %d 条通知", len(notifications))
	return nil
}
