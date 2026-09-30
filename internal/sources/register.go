package sources

import "github.com/slowlyo/meme/internal/core"

// RegisterAllSources 注册所有内置源到注册中心
func RegisterAllSources(registry *core.Registry, config *Config) {
	// 注册无需认证的源
	registry.Register(NewDoutula())
	registry.Register(NewPdan())
	registry.Register(NewSougou())

	if config != nil {
		// 注册需要认证的源 (如果配置了 Cookie)
		if config.DouyinCookie != "" {
			registry.Register(NewDouyin(config.DouyinCookie))
		}
	}
}

// Config 源配置
type Config struct {
	DouyinCookie string `json:"douyin_cookie" yaml:"douyin_cookie"`
}

// SourceStatus 数据源状态信息结构（用于 Web 端展示全景状态）
type SourceStatus struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	RequiresAuth bool   `json:"requires_auth"`
	Enabled      bool   `json:"enabled"`
	ConfigHint   string `json:"config_hint,omitempty"`
}

// GetAllSourceStatus 获取系统支持的全部源及其当前的启用和配置就绪状态
func GetAllSourceStatus(registry *core.Registry) []SourceStatus {
	// 系统已知并支持的全量源元数据
	known := []struct {
		id          string
		name        string
		description string
		requireAuth bool
		configHint  string
	}{
		{"doutula", "斗图啦", "从 doutupk.com 搜索表情包", false, ""},
		{"sougou", "搜狗表情", "从搜狗图片搜索表情包 (JSON API)", false, ""},
		{"pdan", "胖哒", "从 pdan.com.cn 搜索表情包", false, ""},
		{"douyin", "抖音", "从抖音搜索热门表情包 (需要 sessionid/Cookie)", true, "需要配置 sessionid 或 Cookie"},
	}

	statuses := make([]SourceStatus, 0, len(known))
	visited := make(map[string]bool)

	for _, item := range known {
		visited[item.id] = true
		source, isRegistered := registry.Get(item.id)
		hint := ""

		// 如果已注册，使用已注册实例的信息
		if isRegistered {
			statuses = append(statuses, SourceStatus{
				ID:           source.ID(),
				Name:         source.Name(),
				Description:  source.Description(),
				RequiresAuth: source.RequiresAuth(),
				Enabled:      true,
			})
		} else {
			// 未注册时展示提示
			hint = item.configHint
			statuses = append(statuses, SourceStatus{
				ID:           item.id,
				Name:         item.name,
				Description:  item.description,
				RequiresAuth: item.requireAuth,
				Enabled:      false,
				ConfigHint:   hint,
			})
		}
	}

	// 遍历注册中心中其他未在 known 列表中的自定义源 (例如 mock 源)
	for _, source := range registry.List() {
		if !visited[source.ID()] {
			visited[source.ID()] = true
			statuses = append(statuses, SourceStatus{
				ID:           source.ID(),
				Name:         source.Name(),
				Description:  source.Description(),
				RequiresAuth: source.RequiresAuth(),
				Enabled:      true,
			})
		}
	}

	return statuses
}
