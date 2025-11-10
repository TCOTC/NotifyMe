package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"notifyme/internal/logger"
)

// Ld246Auth ld246 认证
type Ld246Auth struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewLd246Auth 创建新的 ld246 认证
func NewLd246Auth(token string) *Ld246Auth {
	return &Ld246Auth{
		baseURL: "https://ld246.com",
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Login 使用用户名和密码登录（如果需要）
// 注意：根据 API 文档，密码需要使用 MD5 哈希后的值
func (a *Ld246Auth) Login(username, password string) (string, error) {
	loginURL := fmt.Sprintf("%s/api/v2/login", a.baseURL)

	// 根据文档，需要使用 JSON body，密码需要 MD5 哈希
	loginData := map[string]string{
		"userName":     username,
		"userPassword": password, // 注意：实际使用时需要先进行 MD5 哈希
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return "", fmt.Errorf("序列化登录数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NotifyMe/1.0")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码，401 表示需要登录，403 表示权限不足
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("登录失败：用户名或密码错误")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("登录失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code        int    `json:"code"`
		Msg         string `json:"msg"`
		Token       string `json:"token"`       // 成功时才有该值
		UserName    string `json:"userName"`   // 用户名
		NeedCaptcha string `json:"needCaptcha"` // 登录失败次数过多会返回该值
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		if result.NeedCaptcha != "" {
			return "", fmt.Errorf("登录失败: %s (需要验证码)", result.Msg)
		}
		return "", fmt.Errorf("登录失败: %s", result.Msg)
	}

	if result.Token == "" {
		return "", fmt.Errorf("登录失败: 未返回 token")
	}

	a.token = result.Token
	logger.Info("ld246 登录成功")
	return result.Token, nil
}

// ValidateToken 验证 token
func (a *Ld246Auth) ValidateToken(token string) error {
	if token == "" {
		return fmt.Errorf("token 为空")
	}

	// 使用 token 调用获取当前登录用户详情 API 来验证
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v2/user", a.baseURL), nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 使用 token 格式，而非 Bearer
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("User-Agent", "NotifyMe/1.0")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("token 无效或已过期")
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("权限不足")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token 验证失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserName string `json:"userName"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("token 验证失败: %s", result.Msg)
	}

	logger.Infof("ld246 token 验证成功，用户: %s", result.Data.UserName)
	return nil
}

// GetToken 获取当前 token
func (a *Ld246Auth) GetToken() string {
	return a.token
}

// SetToken 设置 token
func (a *Ld246Auth) SetToken(token string) {
	a.token = token
}

