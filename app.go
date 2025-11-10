package main

import (
	"context"
	"os"
	"sync"
	"time"

	"notifyme/internal/config"
	"notifyme/internal/logger"
	"notifyme/internal/scheduler"
	"notifyme/internal/tray"
	"notifyme/pkg/types"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	ctxMu      sync.RWMutex
	config     *types.Config
	scheduler  *scheduler.Scheduler
	shouldQuit bool         // 标志是否应该退出程序
	quitMu     sync.RWMutex // 保护 shouldQuit 的互斥锁
}

// NewApp creates a new App application struct
func NewApp() *App {
	// 先加载配置（或使用默认配置）
	cfg, err := config.Load()
	if err != nil {
		// 配置加载失败时使用默认配置
		cfg = &types.Config{
			PollInterval: 60,
			LogLevel:     "debug",
		}
	}

	// 根据配置初始化日志（始终启用文件日志）
	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "debug"
	}
	if err := logger.Init(logLevel, true); err != nil {
		panic(err)
	}

	// 如果配置加载失败，记录错误
	if err != nil {
		logger.Errorf("加载配置失败: %v", err)
	}

	// 初始化调度器
	sched := scheduler.NewScheduler(cfg)

	app := &App{
		config:    cfg,
		scheduler: sched,
	}

	// 初始化系统托盘
	tray.Init(
		func() {
			// 打开 UI 的回调
			app.ShowWindow()
		},
		func() {
			// 退出程序
			app.Quit()
		},
	)

	// 启动调度器
	if err := sched.Start(); err != nil {
		logger.Errorf("启动调度器失败: %v", err)
	}

	return app
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctxMu.Lock()
	a.ctx = ctx
	a.ctxMu.Unlock()
	logger.Info("应用启动完成")
}

// GetConfig 获取配置
func (a *App) GetConfig() *types.Config {
	return a.config
}

// SaveConfig 保存配置
func (a *App) SaveConfig(cfg *types.Config) error {
	if err := config.Save(cfg); err != nil {
		return err
	}

	// 如果日志级别改变，重新初始化日志系统
	if a.config == nil || a.config.LogLevel != cfg.LogLevel {
		logLevel := cfg.LogLevel
		if logLevel == "" {
			logLevel = "debug"
		}
		if err := logger.Init(logLevel, true); err != nil {
			logger.Errorf("重新初始化日志系统失败: %v", err)
		}
	}

	a.config = cfg
	a.scheduler.UpdateConfig(cfg)
	return nil
}

// GetStatus 获取应用状态
func (a *App) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"running":       a.scheduler.IsRunning(),
		"poll_interval": a.config.PollInterval,
	}
}

// GetRecentNotifications 获取最近的通知列表
func (a *App) GetRecentNotifications() []*types.Notification {
	return a.scheduler.GetRecentNotifications()
}

// TriggerCheck 手动触发检查
func (a *App) TriggerCheck() {
	a.scheduler.TriggerCheck()
}

// ShowWindow 显示窗口
func (a *App) ShowWindow() {
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()

	if ctx != nil {
		// 使用 defer recover 捕获可能的 panic
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("显示窗口时发生错误: %v", r)
			}
		}()
		runtime.WindowShow(ctx)
		logger.Info("显示主窗口")
	} else {
		logger.Warn("无法显示窗口：context 未初始化，可能窗口已关闭或应用未完全启动")
	}
}

// HideWindow 隐藏窗口
func (a *App) HideWindow() {
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()

	if ctx != nil {
		runtime.WindowHide(ctx)
		logger.Info("隐藏主窗口")
	}
}

// Quit 退出程序
func (a *App) Quit() {
	logger.Info("开始退出程序")

	// 设置退出标志，允许 OnBeforeClose 真正退出
	a.quitMu.Lock()
	// 如果已经设置了退出标志，避免重复退出
	if a.shouldQuit {
		a.quitMu.Unlock()
		logger.Warn("退出程序已被调用，跳过重复退出")
		return
	}
	a.shouldQuit = true
	a.quitMu.Unlock()

	// 在 goroutine 中停止调度器，避免阻塞退出流程
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		a.scheduler.Stop()
	}()

	// 等待调度器停止，但设置超时
	select {
	case <-stopDone:
		logger.Info("调度器已停止")
	case <-time.After(5 * time.Second):
		logger.Warn("等待调度器停止超时，继续退出流程")
	}

	// 退出 Wails 应用（先退出 Wails，再退出托盘）
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()

	if ctx != nil {
		// 先退出 Wails 应用
		runtime.Quit(ctx)
		// 等待一小段时间，让 Wails 应用有时间退出
		// 然后退出托盘
		go func() {
			time.Sleep(100 * time.Millisecond)
			tray.Quit()
		}()
	} else {
		logger.Warn("无法退出：context 未初始化，直接退出进程")
		// 如果 context 未初始化，先退出托盘，然后退出进程
		tray.Quit()
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}
}

// ShouldQuit 检查是否应该退出程序
func (a *App) ShouldQuit() bool {
	a.quitMu.RLock()
	defer a.quitMu.RUnlock()
	return a.shouldQuit
}
