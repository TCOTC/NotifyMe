package main

import (
	"context"
	"embed"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notifyme/internal/logger"
	"notifyme/internal/singleinstance"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 先初始化日志（单实例检查需要日志）
	if err := logger.Init("debug", true); err != nil {
		panic(err)
	}

	// 检查单实例
	singleinstance.CheckAndExit()
	defer singleinstance.Unlock()

	// Create an instance of the app structure
	// NewApp 内部会根据配置重新初始化日志
	app := NewApp()

	// 设置信号处理，监听 Ctrl+C (SIGINT) 和 SIGTERM
	// 注意：信号处理必须在 wails.Run() 之前设置，但要在 app 创建之后
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Infof("收到信号: %v，开始退出程序", sig)
		app.Quit()
		// 如果 Quit() 没有成功退出，等待一段时间后强制退出
		time.Sleep(2 * time.Second)
		logger.Warn("程序未能正常退出，强制退出")
		os.Exit(1)
	}()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "NotifyMe",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			// 如果设置了退出标志，允许退出
			if app.ShouldQuit() {
				logger.Info("允许退出程序")
				return false // 返回 false 允许窗口关闭，程序退出
			}
			// 否则隐藏窗口而不是退出，让程序在后台运行
			runtime.WindowHide(ctx)
			return true // 返回 true 阻止窗口关闭
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		panic(err)
	}
}
