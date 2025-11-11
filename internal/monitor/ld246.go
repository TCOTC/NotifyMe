package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"notifyme/internal/logger"
	"notifyme/pkg/types"
)

// articleState 帖子状态信息
type articleState struct {
	LastUpdateTime int64 `json:"lastUpdateTime"` // 最后更新时间
	CommentCount   int   `json:"commentCount"`   // 评论数
}

// Ld246Monitor ld246 监控器
type Ld246Monitor struct {
	baseURL               string
	token                 string
	httpClient            *http.Client
	seenArticles          map[string]*articleState // 已见过的帖子状态（ID -> 状态）
	seenArticlesMu        sync.RWMutex             // 保护 seenArticles 的互斥锁
	seenMessages          map[string]bool          // 已见过的消息 ID 集合
	seenMessagesMu        sync.RWMutex             // 保护 seenMessages 的互斥锁
	stateFilePath         string                   // 状态文件路径
	messagesStateFilePath string                   // 消息状态文件路径
}

// NewLd246Monitor 创建新的 ld246 监控器
func NewLd246Monitor(token string) *Ld246Monitor {
	m := &Ld246Monitor{
		baseURL:      "https://ld246.com",
		token:        token,
		seenArticles: make(map[string]*articleState),
		seenMessages: make(map[string]bool),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// 初始化状态文件路径
	m.stateFilePath = m.getStateFilePath()
	m.messagesStateFilePath = m.getMessagesStateFilePath()

	// 加载已见过的帖子状态列表
	m.loadSeenArticles()

	// 加载已见过的消息列表
	m.loadSeenMessages()

	return m
}

// getStateFilePath 获取状态文件路径
func (m *Ld246Monitor) getStateFilePath() string {
	// 优先使用当前目录（与配置文件逻辑保持一致）
	stateDir := filepath.Join(".", "data")
	if _, err := os.Stat(stateDir); err == nil {
		return filepath.Join(stateDir, "ld246_seen_articles.json")
	}

	// 如果当前目录不存在，使用用户配置目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dataDir := filepath.Join(homeDir, ".notifyme", "data")
		os.MkdirAll(dataDir, 0755)
		return filepath.Join(dataDir, "ld246_seen_articles.json")
	}

	// 如果无法获取用户目录，使用当前目录（即使不存在也会在保存时创建）
	return filepath.Join(stateDir, "ld246_seen_articles.json")
}

// getMessagesStateFilePath 获取消息状态文件路径
func (m *Ld246Monitor) getMessagesStateFilePath() string {
	// 优先使用当前目录（与配置文件逻辑保持一致）
	stateDir := filepath.Join(".", "data")
	if _, err := os.Stat(stateDir); err == nil {
		return filepath.Join(stateDir, "ld246_seen_messages.json")
	}

	// 如果当前目录不存在，使用用户配置目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dataDir := filepath.Join(homeDir, ".notifyme", "data")
		os.MkdirAll(dataDir, 0755)
		return filepath.Join(dataDir, "ld246_seen_messages.json")
	}

	// 如果无法获取用户目录，使用当前目录（即使不存在也会在保存时创建）
	return filepath.Join(stateDir, "ld246_seen_messages.json")
}

// loadSeenArticles 从文件加载已见过的帖子状态列表
func (m *Ld246Monitor) loadSeenArticles() {
	m.seenArticlesMu.Lock()
	defer m.seenArticlesMu.Unlock()

	// 如果文件不存在，使用空的 map
	if _, err := os.Stat(m.stateFilePath); os.IsNotExist(err) {
		m.seenArticles = make(map[string]*articleState)
		return
	}

	data, err := os.ReadFile(m.stateFilePath)
	if err != nil {
		logger.Warnf("读取 ld246 已见过帖子状态列表失败: %v，将使用空列表", err)
		m.seenArticles = make(map[string]*articleState)
		return
	}

	var articles map[string]*articleState
	if err := json.Unmarshal(data, &articles); err != nil {
		logger.Warnf("解析 ld246 已见过帖子状态列表失败: %v，将使用空列表", err)
		m.seenArticles = make(map[string]*articleState)
		return
	}

	m.seenArticles = articles
	if m.seenArticles == nil {
		m.seenArticles = make(map[string]*articleState)
	}

	logger.Debugf("已加载 %d 个已见过的 ld246 帖子状态", len(m.seenArticles))
}

// saveSeenArticles 保存已见过的帖子状态列表到文件
func (m *Ld246Monitor) saveSeenArticles() {
	m.seenArticlesMu.RLock()
	defer m.seenArticlesMu.RUnlock()

	data, err := json.MarshalIndent(m.seenArticles, "", "  ")
	if err != nil {
		logger.Errorf("序列化 ld246 已见过帖子状态列表失败: %v", err)
		return
	}

	// 确保目录存在
	stateDir := filepath.Dir(m.stateFilePath)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		logger.Errorf("创建状态文件目录失败: %v", err)
		return
	}

	if err := os.WriteFile(m.stateFilePath, data, 0644); err != nil {
		logger.Errorf("保存 ld246 已见过帖子状态列表失败: %v", err)
		return
	}

	logger.Debugf("已保存 %d 个已见过的 ld246 帖子状态到文件", len(m.seenArticles))
}

// loadSeenMessages 从文件加载已见过的消息 ID 列表
func (m *Ld246Monitor) loadSeenMessages() {
	m.seenMessagesMu.Lock()
	defer m.seenMessagesMu.Unlock()

	// 如果文件不存在，使用空的 map
	if _, err := os.Stat(m.messagesStateFilePath); os.IsNotExist(err) {
		m.seenMessages = make(map[string]bool)
		return
	}

	data, err := os.ReadFile(m.messagesStateFilePath)
	if err != nil {
		logger.Warnf("读取 ld246 已见过消息列表失败: %v，将使用空列表", err)
		m.seenMessages = make(map[string]bool)
		return
	}

	var messageIDs []string
	if err := json.Unmarshal(data, &messageIDs); err != nil {
		logger.Warnf("解析 ld246 已见过消息列表失败: %v，将使用空列表", err)
		m.seenMessages = make(map[string]bool)
		return
	}

	// 转换为 map
	m.seenMessages = make(map[string]bool, len(messageIDs))
	for _, id := range messageIDs {
		m.seenMessages[id] = true
	}

	logger.Debugf("已加载 %d 个已见过的 ld246 消息 ID", len(m.seenMessages))
}

// saveSeenMessages 保存已见过的消息 ID 列表到文件
func (m *Ld246Monitor) saveSeenMessages() {
	m.seenMessagesMu.RLock()
	defer m.seenMessagesMu.RUnlock()

	// 转换为数组
	messageIDs := make([]string, 0, len(m.seenMessages))
	for id := range m.seenMessages {
		messageIDs = append(messageIDs, id)
	}

	data, err := json.Marshal(messageIDs)
	if err != nil {
		logger.Errorf("序列化 ld246 已见过消息列表失败: %v", err)
		return
	}

	// 确保目录存在
	stateDir := filepath.Dir(m.messagesStateFilePath)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		logger.Errorf("创建状态文件目录失败: %v", err)
		return
	}

	if err := os.WriteFile(m.messagesStateFilePath, data, 0644); err != nil {
		logger.Errorf("保存 ld246 已见过消息列表失败: %v", err)
		return
	}

	logger.Debugf("已保存 %d 个已见过的 ld246 消息 ID 到文件", len(messageIDs))
}

// FetchRecentReplies 获取最近回帖（按最近回帖排序的最新帖子列表）
func (m *Ld246Monitor) FetchRecentReplies() ([]*types.Notification, error) {
	url := fmt.Sprintf("%s/api/v2/articles/latest/reply?p=1", m.baseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 如果提供了 token，添加到请求头（使用 token 格式，而非 Bearer）
	if m.token != "" {
		req.Header.Set("Authorization", "token "+m.token)
	}
	req.Header.Set("User-Agent", "NotifyMe/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码，401 表示需要登录，403 表示权限不足
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("需要登录，请设置有效的 API Token")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("权限不足")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 读取响应体内容用于日志记录
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 记录原始响应内容（用于调试）
	// logger.Debugf("ld246 API 响应内容: %s", string(bodyBytes))

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Pagination struct {
				PaginationPageCount int   `json:"paginationPageCount"`
				PaginationPageNums  []int `json:"paginationPageNums"`
			} `json:"pagination"`
			Articles []struct {
				OID                   string `json:"oId"`                   // 帖子 ID
				ArticleTitle          string `json:"articleTitle"`          // 帖子标题
				ArticlePreviewContent string `json:"articlePreviewContent"` // 帖子预览内容
				ArticleAuthorName     string `json:"articleAuthorName"`     // 作者名称
				ArticleCreateTime     int64  `json:"articleCreateTime"`     // 创建时间
				ArticleUpdateTime     int64  `json:"articleUpdateTime"`     // 更新时间
				ArticleCommentCount   int    `json:"articleCommentCount"`   // 评论数
			} `json:"articles"`
		} `json:"data"`
		Msg string `json:"msg"`
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		logger.Errorf("ld246 响应解析失败，原始响应: %s", string(bodyBytes))
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API 返回错误: %s", apiResp.Msg)
	}

	// 收集最新列表中的所有帖子 ID
	currentArticleIDs := make(map[string]bool, len(apiResp.Data.Articles))
	for _, item := range apiResp.Data.Articles {
		currentArticleIDs[item.OID] = true
	}

	// 对比已见过的帖子，只返回新帖子或有新回帖的帖子
	m.seenArticlesMu.RLock()
	newNotifications := make([]*types.Notification, 0)
	updatedArticles := make(map[string]*articleState)
	newArticleCount := 0
	updatedArticleCount := 0

	logger.Debugf("ld246: 当前已见过 %d 个帖子，API 返回 %d 个帖子", len(m.seenArticles), len(apiResp.Data.Articles))

	for _, item := range apiResp.Data.Articles {
		// 使用更新时间，如果没有则使用创建时间
		timeValue := item.ArticleUpdateTime
		if timeValue == 0 {
			timeValue = item.ArticleCreateTime
		}

		// 检查是否是新帖子或有新回帖
		seenState, exists := m.seenArticles[item.OID]
		isNew := false

		if !exists {
			// 新帖子
			isNew = true
			newArticleCount++
			logger.Debugf("ld246: 发现新帖子 ID=%s, 标题=%s", item.OID, item.ArticleTitle)
		} else {
			// 检查是否有新回帖（更新时间或评论数变化）
			if timeValue > seenState.LastUpdateTime || item.ArticleCommentCount > seenState.CommentCount {
				isNew = true
				updatedArticleCount++
				logger.Debugf("ld246: 发现帖子有新回帖 ID=%s, 标题=%s, 更新时间变化=%v, 评论数变化=%v",
					item.OID, item.ArticleTitle,
					timeValue > seenState.LastUpdateTime,
					item.ArticleCommentCount > seenState.CommentCount)
			} else {
				logger.Debugf("ld246: 帖子无变化，跳过 ID=%s, 标题=%s, 当前时间=%d, 已记录时间=%d, 当前评论数=%d, 已记录评论数=%d",
					item.OID, item.ArticleTitle,
					timeValue, seenState.LastUpdateTime,
					item.ArticleCommentCount, seenState.CommentCount)
			}
		}

		if !isNew {
			// 没有变化，跳过
			continue
		}

		// 限制内容长度（移除 HTML 标签）
		content := truncateString(stripHTML(item.ArticlePreviewContent), 100)
		if content == "" {
			content = item.ArticleTitle
		}

		// 构建通知标题
		title := item.ArticleTitle
		hasNewReply := exists && (timeValue > seenState.LastUpdateTime || item.ArticleCommentCount > seenState.CommentCount)
		if hasNewReply {
			title = fmt.Sprintf("有新回帖: %s", item.ArticleTitle)
		}

		// 生成通知 ID：如果是新回帖，在 ID 中包含更新时间戳，确保每次新回帖都会生成新的通知 ID
		notificationID := fmt.Sprintf("ld246_article_%s", item.OID)
		if hasNewReply {
			// 有新回帖时，使用更新时间戳生成新的通知 ID，确保每次新回帖都会触发通知
			notificationID = fmt.Sprintf("ld246_article_%s_%d", item.OID, timeValue)
		}

		notification := &types.Notification{
			ID:      notificationID,
			Title:   title,
			Content: content,
			Link:    fmt.Sprintf("%s/article/%s", m.baseURL, item.OID),
			Source:  "ld246",
			Time:    timeValue,
		}
		newNotifications = append(newNotifications, notification)

		// 记录更新后的状态
		updatedArticles[item.OID] = &articleState{
			LastUpdateTime: timeValue,
			CommentCount:   item.ArticleCommentCount,
		}
	}
	m.seenArticlesMu.RUnlock()

	// 更新已见过的帖子状态列表
	m.seenArticlesMu.Lock()

	// 更新或添加新帖子状态（包括没有变化的帖子）
	for id, state := range updatedArticles {
		m.seenArticles[id] = state
	}

	// 对于没有变化的帖子，也需要更新状态（确保状态文件包含所有当前列表中的帖子）
	for _, item := range apiResp.Data.Articles {
		// 使用更新时间，如果没有则使用创建时间
		timeValue := item.ArticleUpdateTime
		if timeValue == 0 {
			timeValue = item.ArticleCreateTime
		}

		// 如果不在 updatedArticles 中，说明没有变化，但仍需要更新状态
		if _, exists := updatedArticles[item.OID]; !exists {
			m.seenArticles[item.OID] = &articleState{
				LastUpdateTime: timeValue,
				CommentCount:   item.ArticleCommentCount,
			}
		}
	}

	// 移除不在最新列表中的帖子
	removedCount := 0
	for id := range m.seenArticles {
		if !currentArticleIDs[id] {
			delete(m.seenArticles, id)
			removedCount++
		}
	}

	m.seenArticlesMu.Unlock()

	// 无论是否有新帖子，都保存到文件（确保状态文件与当前列表同步）
	if removedCount > 0 {
		logger.Debugf("ld246: 移除了 %d 个不在最新列表中的帖子记录", removedCount)
	}
	m.saveSeenArticles()

	logger.Infof("ld246: 获取到 %d 条最近回帖的帖子，其中 %d 条是新帖子（%d 条全新帖子，%d 条有新回帖）",
		len(apiResp.Data.Articles), len(newNotifications), newArticleCount, updatedArticleCount)
	return newNotifications, nil
}

// FetchUnreadMessages 获取未读消息（获取收到的回帖、提及我的等消息）
func (m *Ld246Monitor) FetchUnreadMessages() ([]*types.Notification, error) {
	logger.Debug("开始获取 ld246 未读消息...")

	// 获取未读消息计数
	countURL := fmt.Sprintf("%s/api/v2/notifications/unread/count", m.baseURL)
	req, err := http.NewRequest("GET", countURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 如果提供了 token，添加到请求头（使用 token 格式，而非 Bearer）
	if m.token != "" {
		req.Header.Set("Authorization", "token "+m.token)
	} else {
		logger.Warn("ld246 token 为空，可能无法获取未读消息")
	}
	req.Header.Set("User-Agent", "NotifyMe/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		logger.Errorf("ld246 未读消息计数请求失败: %v", err)
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("需要登录，请设置有效的 API Token")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("权限不足")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Errorf("ld246 未读消息计数 API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 读取响应体内容用于日志记录
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 记录原始响应内容（用于调试）
	logger.Debugf("ld246 未读消息计数 API 响应内容: %s", string(bodyBytes))

	var countResp struct {
		Code int `json:"code"`
		Data struct {
			UnreadNotificationCnt            int `json:"unreadNotificationCnt"`            // 总未读消息数
			UnreadCommentedNotificationCnt   int `json:"unreadCommentedNotificationCnt"`   // 收到的回帖
			UnreadAtNotificationCnt          int `json:"unreadAtNotificationCnt"`          // 提及我的
			UnreadReplyNotificationCnt       int `json:"unreadReplyNotificationCnt"`       // 收到的回复
			UnreadComment2edNotificationCnt  int `json:"unreadComment2edNotificationCnt"`  // 收到的评论
			UnreadChatNotificationCnt        int `json:"unreadChatNotificationCnt"`        // 聊天消息
			UnreadFollowingNotificationCnt   int `json:"unreadFollowingNotificationCnt"`   // 我关注的
			UnreadPointNotificationCnt       int `json:"unreadPointNotificationCnt"`       // 积分消息
			UnreadWalletNotificationCnt      int `json:"unreadWalletNotificationCnt"`      // 钱包消息
			UnreadBroadcastNotificationCnt   int `json:"unreadBroadcastNotificationCnt"`   // 同城广播
			UnreadSysAnnounceNotificationCnt int `json:"unreadSysAnnounceNotificationCnt"` // 系统公告
			UnreadNewFollowerNotificationCnt int `json:"unreadNewFollowerNotificationCnt"` // 新关注者
			UnreadReviewNotificationCnt      int `json:"unreadReviewNotificationCnt"`      // 审核消息
		} `json:"data"`
		Msg string `json:"msg"`
	}

	if err := json.Unmarshal(bodyBytes, &countResp); err != nil {
		logger.Errorf("ld246 未读消息计数响应解析失败: %v，原始响应: %s", err, string(bodyBytes))
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if countResp.Code != 0 {
		logger.Errorf("ld246 未读消息计数 API 返回错误: code=%d, msg=%s", countResp.Code, countResp.Msg)
		return nil, fmt.Errorf("API 返回错误: %s", countResp.Msg)
	}

	logger.Infof("ld246 未读消息计数详情: 总未读=%d, 收到的回帖=%d, 提及我的=%d, 收到的回复=%d, 聊天=%d, 我关注的=%d",
		countResp.Data.UnreadNotificationCnt,
		countResp.Data.UnreadCommentedNotificationCnt,
		countResp.Data.UnreadAtNotificationCnt,
		countResp.Data.UnreadReplyNotificationCnt,
		countResp.Data.UnreadChatNotificationCnt,
		countResp.Data.UnreadFollowingNotificationCnt)

	// 如果总未读消息数为 0，直接返回
	if countResp.Data.UnreadNotificationCnt == 0 {
		logger.Debug("ld246 总未读消息数为 0，直接返回")
		return []*types.Notification{}, nil
	}

	// 根据各类型的未读数量，获取对应类型的消息
	notifications := []*types.Notification{}

	// 获取收到的回帖消息
	if countResp.Data.UnreadCommentedNotificationCnt > 0 {
		logger.Debug("开始获取 ld246 收到的回帖消息...")
		commentedNotifications, err := m.fetchNotificationsByType("commented")
		if err != nil {
			logger.Errorf("获取收到的回帖消息失败: %v", err)
		} else {
			logger.Debugf("ld246 收到的回帖消息: 获取到 %d 条新消息", len(commentedNotifications))
			notifications = append(notifications, commentedNotifications...)
		}
	}

	// 获取提及我的消息
	if countResp.Data.UnreadAtNotificationCnt > 0 {
		logger.Debug("开始获取 ld246 提及我的消息...")
		atNotifications, err := m.fetchNotificationsByType("at")
		if err != nil {
			logger.Errorf("获取提及我的消息失败: %v", err)
		} else {
			logger.Debugf("ld246 提及我的消息: 获取到 %d 条新消息", len(atNotifications))
			notifications = append(notifications, atNotifications...)
		}
	}

	// 获取收到的回复消息
	if countResp.Data.UnreadReplyNotificationCnt > 0 {
		logger.Debug("开始获取 ld246 收到的回复消息...")
		replyNotifications, err := m.fetchNotificationsByType("reply")
		if err != nil {
			logger.Errorf("获取收到的回复消息失败: %v", err)
		} else {
			logger.Debugf("ld246 收到的回复消息: 获取到 %d 条新消息", len(replyNotifications))
			notifications = append(notifications, replyNotifications...)
		}
	}

	// 获取我关注的消息
	if countResp.Data.UnreadFollowingNotificationCnt > 0 {
		logger.Debug("开始获取 ld246 我关注的消息...")
		followingNotifications, err := m.fetchNotificationsByType("following")
		if err != nil {
			logger.Errorf("获取我关注的消息失败: %v", err)
		} else {
			logger.Debugf("ld246 我关注的消息: 获取到 %d 条新消息", len(followingNotifications))
			notifications = append(notifications, followingNotifications...)
		}
	}

	// 处理聊天消息（根据未读数量生成通知，不通过 API 获取详情）
	currentCount := countResp.Data.UnreadChatNotificationCnt

	if currentCount > 0 {
		logger.Debugf("ld246 检测到 %d 条聊天消息", currentCount)

		// 只要数量 > 0，就生成通知
		chatNotification := &types.Notification{
			ID:      fmt.Sprintf("ld246_chat_%d", currentCount),
			Title:   fmt.Sprintf("聊天消息 (%d)", currentCount),
			Content: fmt.Sprintf("您有 %d 条未读聊天消息", currentCount),
			Link:    "https://ld246.com/chats",
			Source:  "ld246",
			Time:    time.Now().UnixMilli(),
		}
		notifications = append(notifications, chatNotification)

		logger.Debugf("ld246 聊天消息: 生成 %d 条聊天消息通知", currentCount)
	}

	logger.Infof("ld246: 总共获取到 %d 条新未读消息（总未读=%d: 收到的回帖=%d, 提及我的=%d, 收到的回复=%d, 我关注的=%d, 聊天=%d）",
		len(notifications),
		countResp.Data.UnreadNotificationCnt,
		countResp.Data.UnreadCommentedNotificationCnt,
		countResp.Data.UnreadAtNotificationCnt,
		countResp.Data.UnreadReplyNotificationCnt,
		countResp.Data.UnreadFollowingNotificationCnt,
		countResp.Data.UnreadChatNotificationCnt)
	return notifications, nil
}

// fetchNotificationsByType 根据类型获取通知消息
func (m *Ld246Monitor) fetchNotificationsByType(notificationType string) ([]*types.Notification, error) {
	url := fmt.Sprintf("%s/api/v2/notifications/%s?p=1", m.baseURL, notificationType)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 如果提供了 token，添加到请求头（使用 token 格式，而非 Bearer）
	if m.token != "" {
		req.Header.Set("Authorization", "token "+m.token)
	}
	req.Header.Set("User-Agent", "NotifyMe/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("需要登录，请设置有效的 API Token")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("权限不足")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 读取响应体内容用于日志记录
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 记录原始响应内容（用于调试）
	logger.Debugf("ld246 %s 通知 API 响应内容: %s", notificationType, string(bodyBytes))

	var apiResp struct {
		Code int `json:"code"`
		Data []struct {
			ID          string `json:"id"`
			Msg         string `json:"msg"`         // 消息内容
			DataType    int    `json:"dataType"`    // 数据类型
			DataID      string `json:"dataId"`      // 关联数据 ID
			CreatedTime int64  `json:"createdTime"` // 创建时间
			HasRead     bool   `json:"hasRead"`     // 是否已读
		} `json:"data"`
		Msg string `json:"msg"`
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		logger.Errorf("ld246 %s 通知响应解析失败，原始响应: %s", notificationType, string(bodyBytes))
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API 返回错误: %s", apiResp.Msg)
	}

	logger.Debugf("ld246 %s 通知: API 返回 %d 条消息", notificationType, len(apiResp.Data))

	// 对比已见过的消息，只返回新的未读消息
	m.seenMessagesMu.RLock()
	seenCount := len(m.seenMessages)
	logger.Debugf("ld246 %s 通知: 当前已见过 %d 条消息", notificationType, seenCount)

	newNotifications := make([]*types.Notification, 0)
	newMessageIDs := make([]string, 0)
	readCount := 0
	seenMessageCount := 0

	for i, item := range apiResp.Data {
		logger.Debugf("ld246 %s 通知: 处理第 %d 条消息，ID=%s, HasRead=%v, Msg=%s",
			notificationType, i+1, item.ID, item.HasRead, truncateString(item.Msg, 50))

		// 只处理未读消息
		if item.HasRead {
			readCount++
			logger.Debugf("ld246 %s 通知: 消息 ID=%s 已读，跳过", notificationType, item.ID)
			continue
		}

		// 检查是否已见过
		messageID := fmt.Sprintf("%s_%s", notificationType, item.ID)
		if m.seenMessages[messageID] {
			seenMessageCount++
			logger.Debugf("ld246 %s 通知: 消息 ID=%s (messageID=%s) 已见过，跳过",
				notificationType, item.ID, messageID)
			continue
		}

		logger.Debugf("ld246 %s 通知: 发现新消息 ID=%s (messageID=%s)",
			notificationType, item.ID, messageID)

		// 限制内容长度
		content := truncateString(item.Msg, 100)

		// 根据通知类型设置标题
		title := "新消息"
		if notificationType == "commented" {
			title = "收到回帖"
		} else if notificationType == "at" {
			title = "提及我的"
		} else if notificationType == "reply" {
			title = "收到回复"
		} else if notificationType == "following" {
			title = "我关注的"
		} else if notificationType == "chat" {
			title = "聊天消息"
		}

		// 根据 dataType 构建链接
		link := m.baseURL
		if item.DataID != "" {
			// 根据 dataType 判断是帖子还是回帖
			if item.DataType == 3 || item.DataType == 33 || item.DataType == 34 || item.DataType == 35 {
				// 回帖，需要先获取帖子 ID（这里简化处理，直接使用 dataId）
				link = fmt.Sprintf("%s/article/%s", m.baseURL, item.DataID)
			} else if item.DataType == 4 || item.DataType == 9 || item.DataType == 15 || item.DataType == 16 || item.DataType == 20 || item.DataType == 22 {
				// 帖子
				link = fmt.Sprintf("%s/article/%s", m.baseURL, item.DataID)
			}
		}

		notification := &types.Notification{
			ID:      fmt.Sprintf("ld246_%s_%s", notificationType, item.ID),
			Title:   title,
			Content: content,
			Link:    link,
			Source:  "ld246",
			Time:    item.CreatedTime,
		}
		newNotifications = append(newNotifications, notification)
		newMessageIDs = append(newMessageIDs, messageID)
	}
	m.seenMessagesMu.RUnlock()

	// 更新已见过的消息 ID 列表
	if len(newMessageIDs) > 0 {
		m.seenMessagesMu.Lock()
		for _, id := range newMessageIDs {
			m.seenMessages[id] = true
		}
		m.seenMessagesMu.Unlock()

		// 保存到文件
		m.saveSeenMessages()
	}

	logger.Infof("ld246 %s 通知: 统计 - 总消息数=%d, 已读消息=%d, 已见过消息=%d, 新消息=%d",
		notificationType, len(apiResp.Data), readCount, seenMessageCount, len(newNotifications))

	if readCount > 0 {
		logger.Debugf("ld246 %s 通知: 跳过了 %d 条已读消息", notificationType, readCount)
	}
	if seenMessageCount > 0 {
		logger.Debugf("ld246 %s 通知: 跳过了 %d 条已见过的消息", notificationType, seenMessageCount)
	}
	logger.Infof("ld246 %s 通知: 返回 %d 条新未读消息（共 %d 条未读消息）", notificationType, len(newNotifications), len(apiResp.Data)-readCount)

	return newNotifications, nil
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// stripHTML 移除 HTML 标签
func stripHTML(html string) string {
	// 简单的 HTML 标签移除（使用正则表达式会更准确，但这里为了简单使用字符串替换）
	result := html
	// 移除常见的 HTML 标签
	for {
		start := -1
		end := -1
		for i := 0; i < len(result); i++ {
			if result[i] == '<' {
				start = i
			}
			if start >= 0 && result[i] == '>' {
				end = i
				break
			}
		}
		if start >= 0 && end > start {
			result = result[:start] + result[end+1:]
		} else {
			break
		}
	}
	// 解码 HTML 实体
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	return strings.TrimSpace(result)
}
