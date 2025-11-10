package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"notifyme/internal/logger"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// GitHubAuth GitHub 认证
type GitHubAuth struct {
	config *oauth2.Config
	token  *oauth2.Token
}

// NewGitHubAuth 创建新的 GitHub 认证
func NewGitHubAuth(clientID, clientSecret, redirectURL string) *GitHubAuth {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"notifications"},
		Endpoint:     github.Endpoint,
	}

	return &GitHubAuth{
		config: config,
	}
}

// GetAuthURL 获取授权 URL
func (a *GitHubAuth) GetAuthURL(state string) string {
	return a.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// GetAuthURLWithRedirect 获取授权 URL（使用自定义回调 URL）
func (a *GitHubAuth) GetAuthURLWithRedirect(state, redirectURL string) string {
	// 临时修改回调 URL
	originalRedirectURL := a.config.RedirectURL
	a.config.RedirectURL = redirectURL
	url := a.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	a.config.RedirectURL = originalRedirectURL
	return url
}

// ExchangeCode 交换授权码获取 token
func (a *GitHubAuth) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("交换授权码失败: %w", err)
	}

	a.token = token
	logger.Info("GitHub OAuth2 认证成功")
	return token, nil
}

// GetToken 获取当前 token
func (a *GitHubAuth) GetToken() *oauth2.Token {
	return a.token
}

// SetToken 设置 token
func (a *GitHubAuth) SetToken(token *oauth2.Token) {
	a.token = token
}

// RefreshToken 刷新 token
func (a *GitHubAuth) RefreshToken(ctx context.Context) (*oauth2.Token, error) {
	if a.token == nil {
		return nil, fmt.Errorf("token 未设置")
	}

	tokenSource := a.config.TokenSource(ctx, a.token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("刷新 token 失败: %w", err)
	}

	a.token = newToken
	logger.Info("GitHub token 已刷新")
	return newToken, nil
}

// ValidatePAT 验证 Personal Access Token
func ValidatePAT(token string) error {
	if token == "" {
		return fmt.Errorf("token 为空")
	}

	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token 验证失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	logger.Infof("GitHub PAT 验证成功，用户: %s", user.Login)
	return nil
}

// StartOAuth2Server 启动 OAuth2 回调服务器（用于获取授权码）
// 返回服务器实例，以便后续可以关闭
func StartOAuth2Server(port int, authCodeChan chan<- string) (*http.Server, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "未找到授权码", http.StatusBadRequest)
			return
		}

		authCodeChan <- code

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			<html>
			<head><title>授权成功</title></head>
			<body>
				<h1>授权成功！</h1>
				<p>您可以关闭此窗口。</p>
			</body>
			</html>
		`))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("OAuth2 服务器启动失败: %v", err)
		}
	}()

	logger.Infof("OAuth2 回调服务器已启动，端口: %d", port)
	return server, nil
}

// StartOAuth2ServerWithTokenDisplay 启动 OAuth2 回调服务器，在页面中显示 token 供用户复制
func StartOAuth2ServerWithTokenDisplay(port int, authCodeChan chan<- string, githubAuth *GitHubAuth) (*http.Server, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "未找到授权码", http.StatusBadRequest)
			return
		}

		// 交换授权码获取 token
		ctx := r.Context()
		token, err := githubAuth.ExchangeCode(ctx, code)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf(`
				<html>
				<head><title>授权失败</title></head>
				<body>
					<h1>授权失败</h1>
					<p>错误: %s</p>
				</body>
				</html>
			`, err.Error())))
			return
		}

		// 发送授权码到通道（用于自动保存流程）
		select {
		case authCodeChan <- code:
		default:
		}

		// 在页面中显示 token，供用户复制
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`
			<!DOCTYPE html>
			<html lang="zh-CN">
			<head>
				<meta charset="UTF-8">
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
				<title>授权成功</title>
				<style>
					body {
						font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
						max-width: 800px;
						margin: 50px auto;
						padding: 20px;
						background: #f5f5f5;
					}
					.container {
						background: white;
						padding: 30px;
						border-radius: 8px;
						box-shadow: 0 2px 10px rgba(0,0,0,0.1);
					}
					h1 {
						color: #28a745;
						margin-bottom: 20px;
					}
					.token-box {
						background: #f8f9fa;
						border: 2px solid #dee2e6;
						border-radius: 4px;
						padding: 15px;
						margin: 20px 0;
						word-break: break-all;
						font-family: monospace;
						font-size: 14px;
						position: relative;
					}
					.copy-btn {
						background: #007bff;
						color: white;
						border: none;
						padding: 8px 16px;
						border-radius: 4px;
						cursor: pointer;
						margin-top: 10px;
					}
					.copy-btn:hover {
						background: #0056b3;
					}
					.success {
						color: #28a745;
						font-weight: bold;
					}
					.info {
						color: #6c757d;
						margin-top: 20px;
						font-size: 14px;
					}
				</style>
			</head>
			<body>
				<div class="container">
					<h1>✓ 授权成功！</h1>
					<p>请复制下面的 Access Token 并粘贴到应用中的 Token 输入框：</p>
					<div class="token-box" id="token-box">
						%s
					</div>
					<button class="copy-btn" onclick="copyToken()">复制 Token</button>
					<p class="success" id="copy-success" style="display:none;">✓ Token 已复制到剪贴板！</p>
					<div class="info">
						<p><strong>提示：</strong></p>
						<ul>
							<li>复制上面的 Token</li>
							<li>返回应用，将 Token 粘贴到 "Token" 输入框</li>
							<li>选择认证类型为 "OAuth2"</li>
							<li>点击 "保存配置"</li>
						</ul>
					</div>
				</div>
				<script>
					function copyToken() {
						const token = '%s';
						navigator.clipboard.writeText(token).then(function() {
							document.getElementById('copy-success').style.display = 'block';
							setTimeout(function() {
								document.getElementById('copy-success').style.display = 'none';
							}, 3000);
						});
					}
				</script>
			</body>
			</html>
		`, token.AccessToken, token.AccessToken)))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("OAuth2 服务器启动失败: %v", err)
		}
	}()

	logger.Infof("OAuth2 回调服务器已启动（显示 Token 模式），端口: %d", port)
	return server, nil
}

// OpenBrowser 在浏览器中打开 URL
func OpenBrowser(urlStr string) error {
	_, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("无效的 URL: %w", err)
	}

	// 使用系统默认浏览器打开
	return openBrowser(urlStr)
}

// openBrowser 根据操作系统打开浏览器
func openBrowser(urlStr string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", urlStr)
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "linux":
		cmd = exec.Command("xdg-open", urlStr)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开浏览器失败: %w", err)
	}

	return nil
}
