package types

// GitHubAuth 表示 GitHub 认证配置
type GitHubAuth struct {
	Token string `json:"token"` // Personal Access Token
}

// Ld246Config 表示 ld246 认证配置
type Ld246Config struct {
	Token string `json:"token"` // API token
}

// Config 表示应用配置
type Config struct {
	PollInterval int    `json:"poll_interval"` // 轮询间隔（秒），默认 60
	LogLevel     string `json:"log_level"`     // 日志级别：debug, info, warn, error

	// GitHub 认证
	GitHub GitHubAuth `json:"github"`

	// ld246 认证
	Ld246 Ld246Config `json:"ld246"`
}
