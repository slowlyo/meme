package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/slowlyo/meme/internal/core"
)

// DefaultConfigFileName 默认本地配置文件名称
const DefaultConfigFileName = ".config.json"

// LegacyConfigFileName 历史兼容配置文件名称
const LegacyConfigFileName = ".meme-config.json"

// getConfigFilePath 获取配置文件绝对路径
func getConfigFilePath() string {
	// 优先检查当前工作目录下的新版配置文件
	if _, err := os.Stat(DefaultConfigFileName); err == nil {
		return DefaultConfigFileName
	}
	// 其次检查当前目录下的历史旧版配置文件（无缝兼容升级）
	if _, err := os.Stat(LegacyConfigFileName); err == nil {
		return LegacyConfigFileName
	}

	// 再次检查用户主目录 ~/.config/doutu/config.json
	homeDir, err := os.UserHomeDir()
	if err == nil {
		homePath := filepath.Join(homeDir, ".config", "doutu", "config.json")
		if _, err := os.Stat(homePath); err == nil {
			return homePath
		}
	}

	// 默认返回当前目录下的配置文件路径
	return DefaultConfigFileName
}

// LoadConfig 从配置文件或环境变量加载配置信息
func LoadConfig() *Config {
	cfg := &Config{}

	// 尝试从配置文件读取已有配置
	filePath := getConfigFilePath()
	if data, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// 环境变量优先级更高，存在时覆盖文件配置
	if envCookie := strings.TrimSpace(os.Getenv("DOUYIN_COOKIE")); envCookie != "" {
		cfg.DouyinCookie = envCookie
	}

	return cfg
}

// SaveConfig 将配置持久化写入磁盘文件
func SaveConfig(cfg *Config) error {
	filePath := getConfigFilePath()

	// 确保父级目录存在
	dir := filepath.Dir(filePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// ApplyConfig 将最新配置动态应用到注册中心，实现热重载
func ApplyConfig(registry *core.Registry, cfg *Config) {
	// 处理抖音源的动态注册与更新
	if strings.TrimSpace(cfg.DouyinCookie) != "" {
		_ = os.Setenv("DOUYIN_COOKIE", cfg.DouyinCookie)
		// 检查是否已注册抖音源
		if existing, ok := registry.Get("douyin"); ok {
			if dy, isDy := existing.(*DouyinSource); isDy {
				// 已存在时直接更新 Cookie
				dy.SetCookie(cfg.DouyinCookie)
			} else {
				// 类型异常时重新注册
				registry.Register(NewDouyin(cfg.DouyinCookie))
			}
		} else {
			// 未注册时新增注册
			registry.Register(NewDouyin(cfg.DouyinCookie))
		}
	} else {
		_ = os.Unsetenv("DOUYIN_COOKIE")
		// Cookie 为空时注销抖音源
		registry.Unregister("douyin")
	}
}
