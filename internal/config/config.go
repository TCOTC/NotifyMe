package config

import (
	"fmt"
	"os"
	"path/filepath"

	"notifyme/pkg/types"

	"github.com/spf13/viper"
)

const (
	DefaultPollInterval = 60 // 默认轮询间隔 1 分钟
	DefaultLogLevel     = "debug"
	ConfigFileName      = "config.json"
)

var (
	globalConfig *types.Config
)

// Load 加载配置文件
func Load() (*types.Config, error) {
	configPath := getConfigPath()

	viper.SetConfigType("json")
	viper.SetConfigFile(configPath)

	// 设置默认值
	viper.SetDefault("poll_interval", DefaultPollInterval)
	viper.SetDefault("log_level", DefaultLogLevel)
	viper.SetDefault("github.token", "")
	viper.SetDefault("ld246.token", "")

	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := createDefaultConfig(configPath); err != nil {
			return nil, fmt.Errorf("创建默认配置文件失败: %w", err)
		}
	}

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	config := &types.Config{
		// 初始化嵌套字段，确保结构完整（即使配置文件中没有这些字段）
		GitHub: types.GitHubAuth{Token: ""},
		Ld246:  types.Ld246Config{Token: ""},
	}
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 直接读取所有字段的值（viper 的 Unmarshal 可能不会正确填充所有字段）
	// 使用 Get 方法可以正确读取配置文件中的值，如果不存在则使用默认值
	config.PollInterval = viper.GetInt("poll_interval")
	config.LogLevel = viper.GetString("log_level")
	// 直接读取嵌套字段的值（viper 的 Unmarshal 可能不会正确填充嵌套结构）
	config.GitHub.Token = viper.GetString("github.token")
	config.Ld246.Token = viper.GetString("ld246.token")

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	globalConfig = config
	return config, nil
}

// Save 保存配置文件
func Save(config *types.Config) error {
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	configPath := getConfigPath()

	viper.Set("poll_interval", config.PollInterval)
	viper.Set("log_level", config.LogLevel)
	viper.Set("github.token", config.GitHub.Token)
	viper.Set("ld246.token", config.Ld246.Token)

	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	globalConfig = config
	return nil
}

// Get 获取当前配置
func Get() *types.Config {
	if globalConfig == nil {
		// 如果配置未加载，返回默认配置
		return &types.Config{
			PollInterval: DefaultPollInterval,
			LogLevel:     DefaultLogLevel,
			GitHub:       types.GitHubAuth{Token: ""},
			Ld246:        types.Ld246Config{Token: ""},
		}
	}
	return globalConfig
}

// validateConfig 验证配置
func validateConfig(config *types.Config) error {
	if config.PollInterval < 10 {
		return fmt.Errorf("轮询间隔不能小于 10 秒")
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[config.LogLevel] {
		return fmt.Errorf("无效的日志级别: %s", config.LogLevel)
	}

	return nil
}

// getConfigPath 获取配置文件路径
func getConfigPath() string {
	// 优先使用当前目录
	configPath := filepath.Join(".", ConfigFileName)
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}

	// 如果当前目录不存在，使用用户配置目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(homeDir, ".notifyme")
		os.MkdirAll(configDir, 0755)
		return filepath.Join(configDir, ConfigFileName)
	}

	return configPath
}

// createDefaultConfig 创建默认配置文件
func createDefaultConfig(configPath string) error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	defaultConfig := &types.Config{
		PollInterval: DefaultPollInterval,
		LogLevel:     DefaultLogLevel,
	}

	viper.Set("poll_interval", defaultConfig.PollInterval)
	viper.Set("log_level", defaultConfig.LogLevel)
	viper.Set("github.token", "")
	viper.Set("ld246.token", "")

	return viper.WriteConfigAs(configPath)
}
