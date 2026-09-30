package core

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// 常见错误
var (
	ErrSourceNotFound = errors.New("source not found")
	ErrEmptyKeyword   = errors.New("keyword cannot be empty")
	ErrRequestFailed  = errors.New("request failed")
)

// 用于提取 URL 唯一标识的正则
var (
	// 抖音 URL 格式: /tos-cn-i-xxx/ID~...
	douyinURLPattern = regexp.MustCompile(`/tos-cn-i-[^/]+/([^/~?]+)`)
)

// ExtractURLKey 从 URL 中提取唯一标识用于去重
func ExtractURLKey(url string) string {
	// 尝试抖音格式
	if matches := douyinURLPattern.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1]
	}

	// 通用方案：使用 URL 的 MD5
	hash := md5.Sum([]byte(url))
	return hex.EncodeToString(hash[:])
}

// DeduplicateMemes 对表情包列表去重，保持顺序
func DeduplicateMemes(memes []Meme) []Meme {
	seen := make(map[string]bool)
	result := make([]Meme, 0, len(memes))

	for _, meme := range memes {
		key := ExtractURLKey(meme.URL)
		if !seen[key] {
			seen[key] = true
			result = append(result, meme)
		}
	}

	return result
}

// NormalizeURL 标准化 URL
func NormalizeURL(url string) string {
	// 强制 HTTPS
	url = strings.Replace(url, "http://", "https://", 1)

	// 去除尾部空白
	url = strings.TrimSpace(url)

	return url
}

// IsValidImageURL 检查是否是有效的图片 URL
func IsValidImageURL(url string) bool {
	if url == "" {
		return false
	}

	// 必须是 HTTP(S) 协议
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}

	// 检查常见图片扩展名
	lowerURL := strings.ToLower(url)
	validExtensions := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}

	for _, ext := range validExtensions {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}

	// 有些 CDN URL 没有扩展名，但包含图片相关路径
	imageIndicators := []string{"/img/", "/image/", "/pic/", "/photo/", "thumb", "emoji", "sticker"}
	for _, indicator := range imageIndicators {
		if strings.Contains(lowerURL, indicator) {
			return true
		}
	}

	return false
}

// DetectImageFormat 从 URL 检测图片格式
func DetectImageFormat(url string) string {
	lowerURL := strings.ToLower(url)

	switch {
	case strings.Contains(lowerURL, ".gif"):
		return "gif"
	case strings.Contains(lowerURL, ".png"):
		return "png"
	case strings.Contains(lowerURL, ".webp") || strings.Contains(lowerURL, ".awebp"):
		return "webp"
	case strings.Contains(lowerURL, ".jpg") || strings.Contains(lowerURL, ".jpeg"):
		return "jpg"
	default:
		return ""
	}
}
