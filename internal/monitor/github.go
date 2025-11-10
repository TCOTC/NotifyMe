package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"notifyme/internal/logger"
	"notifyme/pkg/types"
)

// GitHubMonitor GitHub 监控器
type GitHubMonitor struct {
	baseURL      string
	token        string
	httpClient   *http.Client
	lastModified time.Time    // 上次查询时间，用于优化轮询
	mu           sync.RWMutex // 保护 lastModified 的互斥锁
}

// NewGitHubMonitor 创建新的 GitHub 监控器
func NewGitHubMonitor(token string) *GitHubMonitor {
	return &GitHubMonitor{
		baseURL: "https://api.github.com",
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchNotifications 获取 GitHub 通知
// 支持 since 查询参数和 Last-Modified 头优化
func (m *GitHubMonitor) FetchNotifications() ([]*types.Notification, error) {
	return m.FetchNotificationsSince(time.Time{})
}

// FetchNotificationsSince 获取自指定时间之后的 GitHub 通知
// 如果 since 为零值，则使用上次查询时间（Last-Modified）
func (m *GitHubMonitor) FetchNotificationsSince(since time.Time) ([]*types.Notification, error) {
	if m.token == "" {
		return nil, fmt.Errorf("GitHub token 未设置")
	}

	// 构建 URL 和查询参数
	apiURL, err := url.Parse(fmt.Sprintf("%s/notifications", m.baseURL))
	if err != nil {
		return nil, fmt.Errorf("解析 URL 失败: %w", err)
	}

	query := apiURL.Query()

	// 确定 since 时间
	m.mu.RLock()
	lastModified := m.lastModified
	m.mu.RUnlock()

	// 如果提供了 since 参数，使用它；否则使用上次查询时间
	if !since.IsZero() {
		lastModified = since
	}

	// 如果 lastModified 不为零，添加 since 查询参数
	if !lastModified.IsZero() {
		// GitHub API 使用 ISO 8601 格式：YYYY-MM-DDTHH:MM:SSZ
		query.Set("since", lastModified.Format(time.RFC3339))
		logger.Debugf("GitHub API 查询参数 since: %s", lastModified.Format(time.RFC3339))
	}

	apiURL.RawQuery = query.Encode()
	reqURL := apiURL.String()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置认证头
	req.Header.Set("Authorization", "token "+m.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// 如果上次查询时间不为零，添加 If-Modified-Since 头进行条件请求
	if !lastModified.IsZero() {
		req.Header.Set("If-Modified-Since", lastModified.UTC().Format(http.TimeFormat))
		logger.Debugf("GitHub API 请求头 If-Modified-Since: %s", lastModified.UTC().Format(http.TimeFormat))
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理 304 Not Modified 响应（没有新通知）
	if resp.StatusCode == http.StatusNotModified {
		logger.Debugf("GitHub API 返回 304 Not Modified，没有新通知")
		// 更新 Last-Modified 时间（从响应头获取，如果没有则使用当前时间）
		if lastModifiedHeader := resp.Header.Get("Last-Modified"); lastModifiedHeader != "" {
			if parsedTime, err := http.ParseTime(lastModifiedHeader); err == nil {
				m.mu.Lock()
				m.lastModified = parsedTime
				m.mu.Unlock()
			}
		}
		return []*types.Notification{}, nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// 更新 Last-Modified 时间（从响应头获取）
	if lastModifiedHeader := resp.Header.Get("Last-Modified"); lastModifiedHeader != "" {
		if parsedTime, err := http.ParseTime(lastModifiedHeader); err == nil {
			m.mu.Lock()
			m.lastModified = parsedTime
			m.mu.Unlock()
			logger.Debugf("GitHub API 响应头 Last-Modified: %s", lastModifiedHeader)
		}
	}

	// 记录 X-Poll-Interval 响应头（轮询间隔建议）
	if pollInterval := resp.Header.Get("X-Poll-Interval"); pollInterval != "" {
		logger.Debugf("GitHub API 响应头 X-Poll-Interval: %s 秒", pollInterval)
	}

	// 格式化并输出原始响应
	// var rawJSON interface{}
	// if err := json.Unmarshal(bodyBytes, &rawJSON); err == nil {
	// 	formattedJSON, err := json.MarshalIndent(rawJSON, "", "  ")
	// 	if err == nil {
	// 		logger.Debugf("GitHub API 原始响应（格式化）:\n%s", string(formattedJSON))
	// 	} else {
	// 		logger.Debugf("GitHub API 原始响应（格式化失败，使用原始）: %s", string(bodyBytes))
	// 	}
	// } else {
	// 	logger.Debugf("GitHub API 原始响应（解析失败，使用原始）: %s", string(bodyBytes))
	// }

	var notifications []struct {
		ID         string `json:"id"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Subject struct {
			Title string `json:"title"`
			Type  string `json:"type"`
			URL   string `json:"url"`
		} `json:"subject"`
		Reason    string    `json:"reason"`
		UpdatedAt time.Time `json:"updated_at"`
		URL       string    `json:"url"`
		HTMLURL   string    `json:"html_url"`
	}

	if err := json.Unmarshal(bodyBytes, &notifications); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	logger.Debugf("GitHub API 返回 %d 条通知", len(notifications))

	result := make([]*types.Notification, 0, len(notifications))
	for i, item := range notifications {
		// 输出每条通知的详细信息（格式化）
		// itemJSON, err := json.MarshalIndent(item, "", "  ")
		// if err == nil {
		// 	logger.Debugf("GitHub 通知 #%d:\n%s", i+1, string(itemJSON))
		// } else {
		// 	logger.Debugf("GitHub 通知 #%d: ID=%s, Repository=%s, Subject.Title=%s, Subject.Type=%s, Subject.URL=%s, URL=%s, HTMLURL=%s",
		// 		i+1, item.ID, item.Repository.FullName, item.Subject.Title, item.Subject.Type, item.Subject.URL, item.URL, item.HTMLURL)
		// }
		// 构建标题
		title := fmt.Sprintf("[%s] %s", item.Repository.FullName, item.Subject.Title)

		// 限制内容长度
		content := truncateString(item.Subject.Title, 100)

		// 将 GitHub API URL 转换为 HTML URL
		// 使用 Subject.URL 来获取实际指向 issue/PR 的链接，而不是通知线程的链接
		link := m.convertGitHubAPIToHTML(item.Subject.URL, item.Subject.Type, item.Repository.FullName)
		if link == "" {
			// 如果 Subject.URL 为空，回退到 HTMLURL（通知线程链接）
			link = item.HTMLURL
		}
		logger.Debugf("GitHub 通知 #%d: Subject.Type=%s, 转换后的链接=%s (Subject.URL=%s, HTMLURL=%s)", i+1, item.Subject.Type, link, item.Subject.URL, item.HTMLURL)

		notification := &types.Notification{
			ID:      fmt.Sprintf("github_%s", item.ID),
			Title:   title,
			Content: content,
			Link:    link,
			Source:  "github",
			Time:    item.UpdatedAt.Unix(),
		}
		result = append(result, notification)
	}

	logger.Infof("GitHub: 获取到 %d 条通知", len(result))
	return result, nil
}

// convertGitHubAPIToHTML 将 GitHub API URL 转换为 HTML URL
func (m *GitHubMonitor) convertGitHubAPIToHTML(apiURL string, subjectType string, repoFullName string) string {
	// GitHub API URL 格式: https://api.github.com/repos/{owner}/{repo}/issues/{number}
	// 或 https://api.github.com/repos/{owner}/{repo}/pulls/{number}
	// 或 https://api.github.com/repos/{owner}/{repo}/releases/{id}
	// HTML URL 格式: https://github.com/{owner}/{repo}/issues/{number}
	// 或 https://github.com/{owner}/{repo}/pull/{number} (注意是 pull 而不是 pulls)
	// 或 https://github.com/{owner}/{repo}/releases/tag/{tag_name} (对于 releases)
	if apiURL == "" {
		return ""
	}
	
	// 对于 Release 类型的通知，需要特殊处理
	if subjectType == "Release" {
		return m.convertReleaseAPIToHTML(apiURL, repoFullName)
	}
	
	// 检查是否是 GitHub API URL
	if len(apiURL) >= 22 && apiURL[:22] == "https://api.github.com" {
		// 去掉 https://api.github.com 前缀
		path := apiURL[22:]
		var htmlURL string
		// 如果路径以 /repos/ 开头，去掉这个前缀
		if len(path) >= 7 && path[:7] == "/repos/" {
			htmlURL = "https://github.com/" + path[7:]
		} else {
			// 如果没有 /repos/ 前缀，直接拼接
			htmlURL = "https://github.com" + path
		}
		// 将 API URL 中的 /pulls/ 替换为 /pull/（GitHub HTML URL 使用单数形式）
		htmlURL = strings.ReplaceAll(htmlURL, "/pulls/", "/pull/")
		return htmlURL
	}
	return apiURL
}

// convertReleaseAPIToHTML 将 Release API URL 转换为 HTML URL
// 需要调用 API 获取 release 详情来获取 tag_name
func (m *GitHubMonitor) convertReleaseAPIToHTML(apiURL string, repoFullName string) string {
	if apiURL == "" {
		return ""
	}
	
	logger.Debugf("处理 Release 类型通知，API URL: %s, Repo: %s", apiURL, repoFullName)
	
	// 调用 API 获取 release 详情
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		logger.Errorf("创建 Release API 请求失败: %v", err)
		return ""
	}
	
	req.Header.Set("Authorization", "token "+m.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := m.httpClient.Do(req)
	if err != nil {
		logger.Errorf("请求 Release API 失败: %v", err)
		return ""
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Errorf("Release API 返回错误状态码 %d: %s", resp.StatusCode, string(bodyBytes))
		return ""
	}
	
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("读取 Release API 响应失败: %v", err)
		return ""
	}
	
	if err := json.Unmarshal(bodyBytes, &release); err != nil {
		logger.Errorf("解析 Release API 响应失败: %v", err)
		return ""
	}
	
	// 优先使用 API 返回的 html_url，如果没有则使用 tag_name 构建
	if release.HTMLURL != "" {
		logger.Debugf("Release API 返回 HTML URL: %s", release.HTMLURL)
		return release.HTMLURL
	}
	
	if release.TagName != "" {
		htmlURL := fmt.Sprintf("https://github.com/%s/releases/tag/%s", repoFullName, release.TagName)
		logger.Debugf("使用 tag_name 构建 Release HTML URL: %s (tag_name=%s)", htmlURL, release.TagName)
		return htmlURL
	}
	
	logger.Warnf("Release API 响应中未找到 tag_name 或 html_url")
	return ""
}
