package main

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
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
	ctx           context.Context
	ctxMu         sync.RWMutex
	config        *types.Config
	scheduler     *scheduler.Scheduler
	shouldQuit    bool         // 标志是否应该退出程序
	quitMu        sync.RWMutex // 保护 shouldQuit 的互斥锁
	showingWindow int32        // 原子标志，表示是否正在显示窗口（0=否，1=是）
	windowVisible int32        // 原子标志，表示窗口是否可见（0=隐藏，1=显示）
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
	// 应用启动时，窗口默认是可见的
	atomic.StoreInt32(&a.windowVisible, 1)
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
	// 快速检查窗口是否已经可见，如果已经可见，只执行必要的操作（如取消最小化、置前）
	// 这样可以避免重复执行可能导致阻塞的操作
	if atomic.LoadInt32(&a.windowVisible) == 1 {
		logger.Debug("窗口已经可见，检查是否需要取消最小化并置前")
		// 窗口已经可见，检查是否最小化，如果最小化则取消最小化，然后置前
		// 在独立的 goroutine 中执行，不阻塞
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Debugf("窗口操作失败: %v", r)
				}
			}()
			a.ctxMu.RLock()
			ctx := a.ctx
			a.ctxMu.RUnlock()
			if ctx == nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				// 先取消最小化（如果窗口被最小化）
				func() {
					defer func() {
						if r := recover(); r != nil {
							// WindowUnminimise 可能不可用，忽略错误
							logger.Debugf("WindowUnminimise 不可用: %v", r)
						}
					}()
					runtime.WindowUnminimise(ctx)
					logger.Debug("WindowUnminimise 调用完成（窗口已可见时）")
				}()

				// 然后执行置前操作
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Debugf("WindowCenter 不可用: %v", r)
						}
					}()
					runtime.WindowCenter(ctx)
					logger.Debug("WindowCenter 调用完成（窗口已可见时）")
				}()
			}
		}()
		return
	}

	// 使用原子操作快速检查是否正在显示窗口，避免重复调用
	// 如果正在执行，直接返回，不阻塞
	if !atomic.CompareAndSwapInt32(&a.showingWindow, 0, 1) {
		logger.Debug("窗口正在显示中，跳过重复调用")
		return
	}

	// 确保在函数退出时重置标志
	defer atomic.StoreInt32(&a.showingWindow, 0)

	logger.Debug("ShowWindow 被调用")

	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()

	if ctx == nil {
		logger.Warn("无法显示窗口：context 未初始化，可能窗口已关闭或应用未完全启动")
		return
	}

	// 检查 context 是否已取消（窗口可能已被关闭）
	select {
	case <-ctx.Done():
		logger.Warn("无法显示窗口：context 已取消，窗口可能已被关闭")
		return
	default:
		// context 仍然有效，继续执行
	}

	// 在 goroutine 中执行窗口操作，避免阻塞调用者
	// 不等待操作完成，立即返回，确保托盘事件处理不被阻塞
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("显示窗口操作发生 panic: %v", r)
				atomic.StoreInt32(&a.showingWindow, 0)
			}
		}()

		// 使用 defer recover 捕获可能的 panic
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("显示窗口时发生错误: %v", r)
			}
		}()

		// 先显示窗口
		runtime.WindowShow(ctx)
		logger.Debug("WindowShow 调用完成")
		atomic.StoreInt32(&a.windowVisible, 1)

		// 如果窗口被最小化，恢复窗口
		// 注意：WindowUnminimise 在某些平台上可能不可用，使用 recover 捕获可能的错误
		func() {
			defer func() {
				if r := recover(); r != nil {
					// WindowUnminimise 可能不可用，忽略错误
					logger.Debugf("WindowUnminimise 不可用: %v", r)
				}
			}()
			runtime.WindowUnminimise(ctx)
			logger.Debug("WindowUnminimise 调用完成")
		}()

		// 将窗口置于前台并获取焦点
		// 注意：WindowCenter 和 WindowFocus 在某些平台上可能不可用
		func() {
			defer func() {
				if r := recover(); r != nil {
					// 这些方法可能不可用，忽略错误
					logger.Debugf("窗口焦点操作不可用: %v", r)
				}
			}()
			runtime.WindowCenter(ctx)
			logger.Debug("WindowCenter 调用完成")
		}()

		atomic.StoreInt32(&a.showingWindow, 0)
		logger.Info("显示主窗口成功")
	}()

	// 不等待操作完成，立即返回，确保托盘事件处理不被阻塞
	logger.Debug("ShowWindow 调用已启动，异步执行中")
}

// HideWindow 隐藏窗口
func (a *App) HideWindow() {
	a.ctxMu.RLock()
	ctx := a.ctx
	a.ctxMu.RUnlock()

	if ctx != nil {
		runtime.WindowHide(ctx)
		atomic.StoreInt32(&a.windowVisible, 0)
		logger.Info("隐藏主窗口")
	}
}

// SetWindowVisible 设置窗口可见状态（供外部调用，如 OnBeforeClose）
func (a *App) SetWindowVisible(visible bool) {
	if visible {
		atomic.StoreInt32(&a.windowVisible, 1)
	} else {
		atomic.StoreInt32(&a.windowVisible, 0)
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
