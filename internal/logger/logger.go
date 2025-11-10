package logger

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	logger *logrus.Logger
)

// fileLogWriter 用于将日志写入文件，使用无颜色格式
type fileLogWriter struct {
	file      *os.File
	formatter *logrus.TextFormatter
}

func (w *fileLogWriter) Write(p []byte) (n int, err error) {
	// 移除 ANSI 颜色代码后写入文件
	cleaned := removeANSICodes(p)
	return w.file.Write(cleaned)
}

// safeMultiWriter 是一个安全的 MultiWriter，即使某个 writer 失败也继续写入其他 writer
type safeMultiWriter struct {
	writers []io.Writer
}

func (w *safeMultiWriter) Write(p []byte) (n int, err error) {
	// 尝试写入所有 writer，即使某个失败也继续
	for _, writer := range w.writers {
		if writer != nil {
			// 忽略错误，确保所有 writer 都尝试写入
			writer.Write(p)
		}
	}
	// 返回成功写入的字节数（假设至少文件写入成功）
	return len(p), nil
}

// removeANSICodes 移除 ANSI 颜色代码
func removeANSICodes(data []byte) []byte {
	// ANSI 转义序列格式: \x1b[数字m 或 \x1b[数字;数字m
	result := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '[' {
			// 找到 ANSI 转义序列，跳过直到 'm'
			i += 2
			for i < len(data) && data[i] != 'm' {
				i++
			}
			if i < len(data) {
				i++ // 跳过 'm'
			}
		} else {
			result = append(result, data[i])
			i++
		}
	}
	return result
}

// Init 初始化日志系统
func Init(logLevel string, logToFile bool) error {
	logger = logrus.New()

	// 设置日志格式
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     true,
	})

	// 设置日志级别
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		level = logrus.DebugLevel // 默认使用 debug 级别
	}
	logger.SetLevel(level)

	// 如果启用文件日志，同时输出到文件和控制台
	if logToFile {
		logDir := getLogDir()
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}

		// 使用日期作为日志文件名，格式：notifyme-2025-01-15.log
		today := time.Now().Format("2006-01-02")
		logFile := filepath.Join(logDir, "notifyme-"+today+".log")
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}

		// 为文件日志创建无颜色的格式化器
		fileFormatter := &logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			DisableColors:   true, // 文件日志不使用颜色
		}

		// 创建自定义 Writer，为文件输出使用无颜色格式
		fileWriter := &fileLogWriter{
			file:      file,
			formatter: fileFormatter,
		}

		// 使用安全的 MultiWriter，即使 stdout 不可用（GUI 模式）也能正常写入文件
		// safeMultiWriter 会尝试写入所有 writer，即使某个失败也继续
		safeWriter := &safeMultiWriter{
			writers: []io.Writer{os.Stdout, fileWriter},
		}
		logger.SetOutput(safeWriter)
	} else {
		logger.SetOutput(os.Stdout)
	}

	return nil
}

// GetLogger 获取日志实例
func GetLogger() *logrus.Logger {
	if logger == nil {
		// 如果未初始化，使用默认配置初始化（启用文件日志）
		Init("debug", true)
	}
	return logger
}

// Debug 记录 debug 级别日志
func Debug(args ...interface{}) {
	GetLogger().Debug(args...)
}

// Debugf 记录格式化 debug 级别日志
func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

// Info 记录 info 级别日志
func Info(args ...interface{}) {
	GetLogger().Info(args...)
}

// Infof 记录格式化 info 级别日志
func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

// Warn 记录 warn 级别日志
func Warn(args ...interface{}) {
	GetLogger().Warn(args...)
}

// Warnf 记录格式化 warn 级别日志
func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

// Error 记录 error 级别日志
func Error(args ...interface{}) {
	GetLogger().Error(args...)
}

// Errorf 记录格式化 error 级别日志
func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

// getLogDir 获取日志目录
func getLogDir() string {
	// 始终使用用户配置目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(homeDir, ".notifyme", "logs")
	}

	// 如果无法获取用户目录，回退到当前目录
	return filepath.Join(".", "logs")
}
