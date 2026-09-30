package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/slowlyo/meme/internal/core"
)

// ============ 搜狗表情 (Sougou) ============

type SougouSource struct {
	BaseSource
}

func NewSougou() *SougouSource {
	return &SougouSource{
		BaseSource: BaseSource{
			id:          "sougou",
			name:        "搜狗表情",
			description: "从搜狗图片搜索表情包 (JSON API)",
			requireAuth: false,
			client:      newHTTPClient(),
		},
	}
}

// sougouResponse 搜狗 PC 版 API 响应结构
// API: https://pic.sogou.com/napi/pc/searchList
type sougouResponse struct {
	Data struct {
		Items []struct {
			LocImageLink string `json:"locImageLink"` // CDN 链接
			ThumbUrl     string `json:"thumbUrl"`     // 缩略图链接
			OriPicUrl    string `json:"oriPicUrl"`    // 原始图片链接（优先使用）
			PicUrl       string `json:"picUrl"`       // 图片页面链接（备选）
			Title        string `json:"title"`        // 标题
			Width        int    `json:"width"`
			Height       int    `json:"height"`
		} `json:"items"`
	} `json:"data"`
	Status int `json:"status"`
}

func (s *SougouSource) Search(ctx context.Context, keyword string, opts core.SearchOptions) ([]core.Meme, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}

	// 计算分页参数
	pageSize := 48
	start := (page - 1) * pageSize

	// 构造新的 API URL
	// tagQSign 是固定的表情包标签签名
	params := url.Values{
		"mode":     {"1"},
		"tagQSign": {"表情包,5e604ff6"},
		"start":    {fmt.Sprintf("%d", start)},
		"xml_len":  {fmt.Sprintf("%d", pageSize)},
		"query":    {keyword},
		"channel":  {"pc_pic"},
		"scene":    {"pic_result"},
	}

	apiURL := "https://pic.sogou.com/napi/pc/searchList?" + params.Encode()
	fmt.Fprintf(os.Stderr, "🌐 [Request] GET %s\n", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 设置完整的浏览器 Headers
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	req.Header.Set("X-Time4p", fmt.Sprintf("%d", time.Now().UnixMilli()))
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("Referer", "https://pic.sogou.com/pics")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data sougouResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode JSON failed: %w", err)
	}

	var memes []core.Meme
	for _, item := range data.Data.Items {
		// 优先使用原始图片链接，其次 CDN 链接，最后缩略图
		imgURL := item.OriPicUrl
		if imgURL == "" {
			imgURL = item.PicUrl
		}
		if imgURL == "" {
			imgURL = item.LocImageLink
		}
		if imgURL == "" {
			imgURL = item.ThumbUrl
		}
		if imgURL == "" {
			continue
		}

		imgURL = core.NormalizeURL(imgURL)
		if !core.IsValidImageURL(imgURL) {
			continue
		}

		title := item.Title
		if title == "" {
			title = "搜狗表情"
		}

		memes = append(memes, core.Meme{
			Title:    title,
			URL:      imgURL,
			Platform: s.id,
			Format:   core.DetectImageFormat(imgURL),
			Width:    item.Width,
			Height:   item.Height,
		})
	}

	if opts.Limit > 0 && len(memes) > opts.Limit {
		memes = memes[:opts.Limit]
	}

	return memes, nil
}

// ============ 抖音 (Douyin) ============

// DouyinSource 抖音表情搜索源实现
// 抖音检索需要两个凭证：用户登录态 sessionid 与设备风控标识 ttwid。
// 本源支持用户仅输入 32 位 sessionid，后端将自动请求官方公开注册端点获取 ttwid 并拼接。
type DouyinSource struct {
	BaseSource
	cookie       string       // 经标准化后的用户基础 Cookie (至少包含 sessionid)
	cachedTTWID  string       // 自动获取的 ttwid 缓存，避免每次搜索都重复注册
	ttwidExpires time.Time    // ttwid 缓存过期时间
	mu           sync.RWMutex // 读写锁，保障多并发检索与动态配置更新下的线程安全
}

// NewDouyin 实例化抖音表情源
func NewDouyin(cookie string) *DouyinSource {
	return &DouyinSource{
		BaseSource: BaseSource{
			id:          "douyin",
			name:        "抖音",
			description: "从抖音搜索热门表情包 (需要 Cookie 或 sessionid)",
			requireAuth: true,
			client:      newHTTPClient(),
		},
		cookie: normalizeDouyinCookie(cookie),
	}
}

// SetCookie 动态更新 Cookie 并规范化格式
func (s *DouyinSource) SetCookie(cookie string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cookie = normalizeDouyinCookie(cookie)
}

// normalizeDouyinCookie 智能规范化用户输入的抖音凭证
// 支持格式：
// 1. 纯 32 位 hex sessionid (例如 996a3db26a82401454f2c8eb1946a815)
// 2. 浏览器复制的 cURL 命令 (自动正则抽取出 sessionid 与 ttwid)
// 3. 完整的请求标头文本或长串 Cookie
func normalizeDouyinCookie(raw string) string {
	raw = strings.TrimSpace(raw)
	// 输入为空直接返回
	if raw == "" {
		return ""
	}

	// 优先检查是否为直接粘贴的纯 32 位 hex 字符串
	if !strings.Contains(raw, "=") && !strings.Contains(raw, ";") && len(raw) >= 30 && len(raw) <= 40 {
		return "sessionid=" + raw
	}

	// 正则提取 sessionid 键值对，兼容长串 Cookie、cURL 以及任意标头文本
	reSession := regexp.MustCompile(`(?i)\bsessionid=([a-zA-Z0-9_-]+)`)
	if match := reSession.FindStringSubmatch(raw); len(match) > 1 {
		sessionVal := match[1]

		// 检查输入文本中是否同时携带了 ttwid，若存在则一并提取
		reTTWID := regexp.MustCompile(`(?i)\bttwid=([^;\s'"]+)`)
		if ttwidMatch := reTTWID.FindStringSubmatch(raw); len(ttwidMatch) > 1 {
			return fmt.Sprintf("sessionid=%s; ttwid=%s", sessionVal, ttwidMatch[1])
		}

		// 若未包含 ttwid，仅保留规范化的 sessionid，后续请求时会自动补齐 ttwid
		return "sessionid=" + sessionVal
	}

	// 去除可能遗留的 "Cookie: " 标头前缀
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "cookie:") {
		raw = strings.TrimSpace(raw[7:])
	}

	return raw
}

// fetchDouyinTTWID 通过字节跳动官方公开注册端点动态获取设备凭证 ttwid
// 该端点用于游客及通用客户端注册，能稳定下发有效的 ttwid 标识
func fetchDouyinTTWID(ctx context.Context, client *http.Client) (string, error) {
	reqBody := []byte(`{"region":"cn","aid":1128,"service":"www.douyin.com"}`)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://ttwid.bytedance.com/ttwid/union/register/", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create ttwid request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute ttwid request failed: %w", err)
	}
	defer resp.Body.Close()

	// 优先从 HTTP Set-Cookie 解析结构中查找
	for _, c := range resp.Cookies() {
		if c.Name == "ttwid" && c.Value != "" {
			return c.Value, nil
		}
	}

	// 降级方案：从 Set-Cookie 响应头文本中正则/前缀提取
	for _, header := range resp.Header["Set-Cookie"] {
		if strings.Contains(header, "ttwid=") {
			parts := strings.Split(header, ";")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(p, "ttwid=") {
					val := strings.TrimPrefix(p, "ttwid=")
					if val != "" {
						return val, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("ttwid not found in response headers")
}

// getEffectiveCookie 获取最终用于请求的完整 Cookie
// 若用户配置中已自带 ttwid，则直接使用；若缺少 ttwid，则自动获取并拼装
func (s *DouyinSource) getEffectiveCookie(ctx context.Context) (string, error) {
	s.mu.RLock()
	currentCookie := s.cookie
	cached := s.cachedTTWID
	expired := time.Now().After(s.ttwidExpires)
	s.mu.RUnlock()

	// 校验基础凭证是否存在
	if currentCookie == "" {
		return "", fmt.Errorf("douyin source requires cookie configuration")
	}

	// 若原始配置中已经包含了 ttwid，直接使用无需自动补全
	if strings.Contains(currentCookie, "ttwid=") {
		return currentCookie, nil
	}

	// 若内存缓存中的 ttwid 未过期，直接拼接复用
	if cached != "" && !expired {
		return currentCookie + "; ttwid=" + cached, nil
	}

	// 缓存未命中或已过期，申请写锁进行获取与刷新
	s.mu.Lock()
	defer s.mu.Unlock()

	// 双重检查锁定，防止高并发下重复请求外部端点
	if s.cachedTTWID != "" && time.Now().Before(s.ttwidExpires) {
		return s.cookie + "; ttwid=" + s.cachedTTWID, nil
	}

	// 发起端点请求获取新的 ttwid
	ttwid, err := fetchDouyinTTWID(ctx, s.client)
	if err != nil {
		// 若获取失败，降级使用用户原始 Cookie 尝试，避免直接阻断
		return s.cookie, nil
	}

	// 成功获取后写入缓存，缓存 24 小时
	s.cachedTTWID = ttwid
	s.ttwidExpires = time.Now().Add(24 * time.Hour)

	return s.cookie + "; ttwid=" + s.cachedTTWID, nil
}

// douyinResponse 抖音 API 响应结构
type douyinResponse struct {
	EmoticonData struct {
		StickerList []struct {
			Author struct {
				Name string `json:"name"`
			} `json:"author"`
			Origin struct {
				URLList []string `json:"url_list"`
			} `json:"origin"`
		} `json:"sticker_list"`
	} `json:"emoticon_data"`
}

// Search 执行抖音表情包检索
func (s *DouyinSource) Search(ctx context.Context, keyword string, opts core.SearchOptions) ([]core.Meme, error) {
	// 获取拼装好的有效鉴权 Cookie
	effectiveCookie, err := s.getEffectiveCookie(ctx)
	if err != nil {
		return nil, err
	}

	page := opts.Page
	// 边界检查：页码最小为 1
	if page < 1 {
		page = 1
	}
	cursor := (page - 1) * 10

	params := url.Values{
		"device_platform": {"webapp"},
		"aid":             {"1128"},
		"keyword":         {keyword},
		"cursor":          {fmt.Sprintf("%d", cursor)},
	}

	apiURL := "https://www.douyin.com/aweme/v1/web/im/resource/emoticon/search?" + params.Encode()
	fmt.Fprintf(os.Stderr, "🌐 [Request] GET %s\n", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.douyin.com/")
	req.Header.Set("Cookie", effectiveCookie)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码是否正常
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "🌐 [Response] %s\n", string(body))

	var data douyinResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("decode JSON failed: %w", err)
	}

	var memes []core.Meme
	for _, item := range data.EmoticonData.StickerList {
		// 跳过无图片 URL 的空项
		if len(item.Origin.URLList) == 0 {
			continue
		}

		imgURL := core.NormalizeURL(item.Origin.URLList[0])

		// 过滤不合法的图片 URL
		if !core.IsValidImageURL(imgURL) {
			continue
		}

		title := item.Author.Name
		// 若作者名为空，填充默认标题
		if title == "" {
			title = "抖音表情"
		}

		memes = append(memes, core.Meme{
			Title:    title,
			URL:      imgURL,
			Platform: s.id,
			Format:   core.DetectImageFormat(imgURL),
		})
	}

	// 若指定了数量上限，进行截断
	if opts.Limit > 0 && len(memes) > opts.Limit {
		memes = memes[:opts.Limit]
	}

	return memes, nil
}
