package web

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slowlyo/meme/internal/core"
	"github.com/slowlyo/meme/internal/sources"
)

//go:embed static/*
var staticFS embed.FS

// Server 表示 Web 服务的核心结构
type Server struct {
	registry    *core.Registry
	httpServer  *http.Server
	proxyClient *http.Client
	host        string
	port        int
}

// NewServer 创建并初始化 Web 服务实例
func NewServer(registry *core.Registry, host string, port int) *Server {
	// 创建用于图片代理的专用 HTTP 客户端，放宽 TLS 并设置合理超时
	proxyClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DialContext: (&net.Dialer{
				Timeout:   8 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     60 * time.Second,
		},
	}

	s := &Server{
		registry:    registry,
		proxyClient: proxyClient,
		host:        host,
		port:        port,
	}

	mux := http.NewServeMux()
	// 注册静态资源首页路由
	mux.HandleFunc("/", s.handleIndex)
	// 注册可用数据源查询接口
	mux.HandleFunc("/api/sources", s.handleSources)
	// 注册表情包聚合搜索接口
	mux.HandleFunc("/api/search", s.handleSearch)
	// 注册防盗链图片代理接口
	mux.HandleFunc("/api/proxy", s.handleProxy)
	// 注册数据源配置查询与更新接口
	mux.HandleFunc("/api/config", s.handleConfig)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// Start 启动 HTTP Web 服务并开始监听连接
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭 Web 服务
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleIndex 处理根路径请求，输出嵌入的前端页面
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// 非根路径请求重定向或返回 404，避免吞掉不存在的路径
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// 从嵌入式文件系统中读取前端 HTML
	content, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		// 嵌入文件读取失败属于程序打包异常，返回 500
		http.Error(w, "Failed to load index page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// handleSources 处理获取所有可用表情包源的信息列表请求
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	// 允许 GET 和 HEAD 请求
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	statuses := sources.GetAllSourceStatus(s.registry)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(statuses)
}

// handleSearch 处理表情包搜索请求，支持关键词、指定源、分页等参数
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	// 允许 GET 和 HEAD 查询
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	query := r.URL.Query()
	keyword := strings.TrimSpace(query.Get("keyword"))
	// 关键词为空时直接返回错误，避免无效抓取
	if keyword == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "keyword is required",
		})
		return
	}

	// 解析指定的数据源列表
	var sourceIDs []string
	if rawSources := strings.TrimSpace(query.Get("sources")); rawSources != "" {
		parts := strings.Split(rawSources, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			// 过滤空项
			if trimmed != "" {
				sourceIDs = append(sourceIDs, trimmed)
			}
		}
	}

	// 解析页码，默认第 1 页
	page := 1
	if p, err := strconv.Atoi(query.Get("page")); err == nil && p > 0 {
		page = p
	}

	// 解析数量限制，默认 20 条，最大不超过 100 条防止内存膨胀
	limit := 20
	if l, err := strconv.Atoi(query.Get("limit")); err == nil && l > 0 {
		if l > 100 {
			limit = 100
		} else {
			limit = l
		}
	}

	opts := core.SearchOptions{
		Page:    page,
		Limit:   limit,
		Timeout: 15 * time.Second,
	}

	// 检查客户端是否要求流式实时输出
	if query.Get("stream") == "1" || query.Get("stream") == "true" {
		s.handleSearchStream(w, r, keyword, sourceIDs, opts)
		return
	}

	var result core.SearchResult
	// 判断是针对指定源检索还是全量源并发检索
	if len(sourceIDs) > 0 {
		result = s.registry.SearchSources(r.Context(), keyword, sourceIDs, opts)
	} else {
		result = s.registry.SearchAll(r.Context(), keyword, opts)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

// streamEvent 表示流式推送时的单源结果数据结构
type streamEvent struct {
	SourceID string      `json:"source_id"`
	Memes    []core.Meme `json:"memes"`
	Error    string      `json:"error,omitempty"`
}

// handleSearchStream 使用 SSE 协议实时向客户端流式推送单个数据源的抓取结果
func (s *Server) handleSearchStream(w http.ResponseWriter, r *http.Request, keyword string, sourceIDs []string, opts core.SearchOptions) {
	flusher, ok := w.(http.Flusher)
	// 如果客户端或代理不支持 Flusher，则降级为普通一次性返回
	if !ok {
		var result core.SearchResult
		if len(sourceIDs) > 0 {
			result = s.registry.SearchSources(r.Context(), keyword, sourceIDs, opts)
		} else {
			result = s.registry.SearchAll(r.Context(), keyword, opts)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	startTime := time.Now()

	// 筛选要并发搜索的目标源列表
	var targets []core.Source
	if len(sourceIDs) > 0 {
		for _, id := range sourceIDs {
			if src, found := s.registry.Get(id); found {
				targets = append(targets, src)
			}
		}
	} else {
		targets = s.registry.List()
	}

	// 若无可用数据源，直接推送完成事件
	if len(targets) == 0 {
		doneData, _ := json.Marshal(map[string]any{"duration_ms": 0, "total": 0})
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
		flusher.Flush()
		return
	}

	resultCh := make(chan streamEvent, len(targets))
	var wg sync.WaitGroup

	// 并发触发各源检索
	for _, src := range targets {
		wg.Add(1)
		go func(target core.Source) {
			defer wg.Done()

			timeout := opts.Timeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}
			sourceCtx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			memes, err := target.Search(sourceCtx, keyword, opts)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}

			resultCh <- streamEvent{
				SourceID: target.ID(),
				Memes:    memes,
				Error:    errStr,
			}
		}(src)
	}

	// 在后台等待全部源完成并关闭结果通道
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 实时消费并流式写出完成的源数据
	for ev := range resultCh {
		data, err := json.Marshal(ev)
		if err == nil {
			fmt.Fprintf(w, "event: source\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}

	// 所有源完成后推送结束事件
	doneData, _ := json.Marshal(map[string]any{
		"duration_ms": time.Since(startTime).Milliseconds(),
	})
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneData)
	flusher.Flush()
}

// handleProxy 处理防盗链图片的代理转发请求
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// 允许 GET 和 HEAD 方法
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	// 目标 URL 不能为空
	if rawURL == "" {
		http.Error(w, "url parameter is required", http.StatusBadRequest)
		return
	}

	targetURL, err := url.Parse(rawURL)
	// 验证 URL 格式是否合法，只允许 http 与 https 协议防止安全风险
	if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") {
		http.Error(w, "invalid target url", http.StatusBadRequest)
		return
	}

	referer := strings.TrimSpace(r.URL.Query().Get("referer"))
	// 如果前端未提供 referer，根据目标 host 自动回退推导默认 Referer
	if referer == "" {
		referer = fmt.Sprintf("%s://%s/", targetURL.Scheme, targetURL.Host)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, rawURL, nil)
	if err != nil {
		http.Error(w, "failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// 伪装浏览器请求头绕过防盗链及安全拦截
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

	resp, err := s.proxyClient.Do(req)
	if err != nil {
		// 上游请求失败直接返回 502 Bad Gateway
		http.Error(w, "failed to fetch upstream image: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 上游状态码非 200 时返回对应错误
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("upstream returned status: %d", resp.StatusCode), resp.StatusCode)
		return
	}

	// 转发 Content-Type 响应头
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		// 兜底二进制流格式
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	// 启用客户端强缓存，减轻重复拉取开销
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)

	// HEAD 请求无需写入响应体
	if r.Method == http.MethodHead {
		return
	}

	// 流式传输图片内容
	_, _ = io.Copy(w, resp.Body)
}

// handleConfig 处理数据源额外配置的查询与热更新
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// 针对 OPTIONS 预检请求直接返回 204
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// GET 方法：获取当前配置
	if r.Method == http.MethodGet {
		cfg := sources.LoadConfig()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"douyin_cookie":     cfg.DouyinCookie,
			"has_douyin_cookie": strings.TrimSpace(cfg.DouyinCookie) != "",
		})
		return
	}

	// POST 方法：保存并应用新配置
	if r.Method == http.MethodPost {
		var req struct {
			DouyinCookie *string `json:"douyin_cookie"`
		}

		// 解析请求体 JSON
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		// 加载基础配置并合并新提交的字段
		cfg := sources.LoadConfig()
		if req.DouyinCookie != nil {
			cfg.DouyinCookie = strings.TrimSpace(*req.DouyinCookie)
		}

		// 持久化保存到配置文件
		if err := sources.SaveConfig(cfg); err != nil {
			http.Error(w, "failed to save config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 动态应用到注册中心生效
		sources.ApplyConfig(s.registry, cfg)

		// 返回成功状态及最新源信息列表
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"message": "配置已保存并生效",
			"sources": sources.GetAllSourceStatus(s.registry),
		})
		return
	}

	// 其它未支持的方法返回 405
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
