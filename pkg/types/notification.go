package types

// Notification 表示一个通知消息
type Notification struct {
	ID      string `json:"id"`      // 唯一标识符
	Title   string `json:"title"`   // 标题
	Content string `json:"content"` // 内容摘要
	Link    string `json:"link"`    // 跳转链接
	Source  string `json:"source"`  // 来源（ld246 或 github）
	Time    int64  `json:"time"`    // 时间戳
}
