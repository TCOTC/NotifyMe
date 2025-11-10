package singleinstance

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
	"notifyme/internal/logger"
)

var (
	mutexHandle windows.Handle
)

// Lock 尝试获取单实例锁
// 如果已经有实例在运行，返回 false
func Lock() (bool, error) {
	name, err := syscall.UTF16PtrFromString("NotifyMe_SingleInstance_Mutex")
	if err != nil {
		return false, fmt.Errorf("创建互斥体名称失败: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if err == windows.ERROR_ALREADY_EXISTS {
			logger.Info("检测到已有实例在运行，退出当前实例")
			return false, nil
		}
		return false, fmt.Errorf("创建互斥体失败: %w", err)
	}

	mutexHandle = handle
	logger.Info("单实例锁已获取")
	return true, nil
}

// Unlock 释放单实例锁
func Unlock() {
	if mutexHandle != 0 {
		windows.CloseHandle(mutexHandle)
		mutexHandle = 0
		logger.Info("单实例锁已释放")
	}
}

// CheckAndExit 检查是否有其他实例在运行，如果有则退出
func CheckAndExit() {
	locked, err := Lock()
	if err != nil {
		logger.Errorf("检查单实例失败: %v", err)
		os.Exit(1)
	}
	if !locked {
		os.Exit(0)
	}
}

