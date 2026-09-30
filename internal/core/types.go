package core

import (
	"context"
	"time"
)

// Meme 表情包数据结构
type Meme struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Platform string `json:"platform"`
	// 可选元数据
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Format string `json:"format,omitempty"` // gif, png, jpg, webp
}

// SearchOptions 搜索选项
type SearchOptions struct {
	Page    int
	Limit   int
	Timeout time.Duration
}

// DefaultSearchOptions 返回默认搜索选项
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		Page:    1,
		Limit:   20,
		Timeout: 10 * time.Second,
	}
}

// Source 数据源接口 - 所有源必须实现此接口
type Source interface {
	// ID 返回源的唯一标识
	ID() string
	// Name 返回源的显示名称
	Name() string
	// Description 返回源的描述
	Description() string
	// Search 执行搜索
	Search(ctx context.Context, keyword string, opts SearchOptions) ([]Meme, error)
	// RequiresAuth 是否需要认证 (如 Cookie)
	RequiresAuth() bool
}

// SearchResult 聚合搜索结果
type SearchResult struct {
	Memes      []Meme            `json:"memes"`
	Sources    []string          `json:"sources"`     // 成功的源
	Errors     map[string]string `json:"errors"`      // 失败的源及原因
	Total      int               `json:"total"`
	DurationMs int64             `json:"duration_ms"`
}
