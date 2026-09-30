package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/slowlyo/meme/internal/core"
)

// mockSource 模拟表情包数据源
type mockSource struct {
	id   string
	name string
}

// ID 返回模拟源的标识
func (m *mockSource) ID() string { return m.id }

// Name 返回模拟源的名称
func (m *mockSource) Name() string { return m.name }

// Description 返回模拟源的描述
func (m *mockSource) Description() string { return "模拟测试源" }

// RequiresAuth 返回是否需要认证
func (m *mockSource) RequiresAuth() bool { return false }

// Search 模拟搜索返回固定数据
func (m *mockSource) Search(ctx context.Context, keyword string, opts core.SearchOptions) ([]core.Meme, error) {
	// 如果关键词为 empty 则返回空列表
	if keyword == "empty" {
		return []core.Meme{}, nil
	}

	return []core.Meme{
		{
			Title:    "测试表情_" + keyword,
			URL:      "https://example.com/test.jpg",
			Platform: m.id,
			Format:   "jpg",
		},
	}, nil
}

// TestHandleIndex 测试根路径首页 HTML 返回
func TestHandleIndex(t *testing.T) {
	registry := core.NewRegistry()
	server := NewServer(registry, "127.0.0.1", 8080)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	// 验证 HTTP 状态码
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// 验证 Content-Type 响应头
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected text/html content-type, got %s", contentType)
	}

	// 测试非根路径 404
	req404 := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	w404 := httptest.NewRecorder()
	server.handleIndex(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w404.Code)
	}
}

// TestHandleSources 测试获取数据源列表接口
func TestHandleSources(t *testing.T) {
	registry := core.NewRegistry()
	registry.Register(&mockSource{id: "mock1", name: "模拟源1"})

	server := NewServer(registry, "127.0.0.1", 8080)

	req := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	w := httptest.NewRecorder()

	server.handleSources(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	// 验证返回结果中包含模拟源
	if !strings.Contains(body, "mock1") {
		t.Errorf("expected response to contain mock1, got %s", body)
	}
}

// TestHandleSearch 测试表情包搜索接口
func TestHandleSearch(t *testing.T) {
	registry := core.NewRegistry()
	registry.Register(&mockSource{id: "mock1", name: "模拟源1"})

	server := NewServer(registry, "127.0.0.1", 8080)

	// 测试缺少关键词时返回 400
	reqEmpty := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	wEmpty := httptest.NewRecorder()
	server.handleSearch(wEmpty, reqEmpty)
	if wEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty keyword, got %d", wEmpty.Code)
	}

	// 测试有效搜索请求
	reqValid := httptest.NewRequest(http.MethodGet, "/api/search?keyword=cat&limit=5", nil)
	wValid := httptest.NewRecorder()
	server.handleSearch(wValid, reqValid)
	if wValid.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", wValid.Code)
	}

	body := wValid.Body.String()
	if !strings.Contains(body, "测试表情_cat") {
		t.Errorf("expected response to contain test meme, got %s", body)
	}
}

// TestHandleSearchStream 测试 SSE 流式搜索接口
func TestHandleSearchStream(t *testing.T) {
	registry := core.NewRegistry()
	registry.Register(&mockSource{id: "mock1", name: "模拟源1"})

	server := NewServer(registry, "127.0.0.1", 8080)

	// 发起流式请求
	reqStream := httptest.NewRequest(http.MethodGet, "/api/search?keyword=cat&stream=1", nil)
	wStream := httptest.NewRecorder()
	server.handleSearch(wStream, reqStream)

	if wStream.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", wStream.Code)
	}

	body := wStream.Body.String()
	// 验证包含 source 事件和 done 事件
	if !strings.Contains(body, "event: source") {
		t.Errorf("expected event: source in stream output, got %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected event: done in stream output, got %s", body)
	}
	if !strings.Contains(body, "测试表情_cat") {
		t.Errorf("expected meme content in stream output, got %s", body)
	}
}

// TestHandleProxyInvalidURL 测试图片代理对非法参数的校验拦截
func TestHandleProxyInvalidURL(t *testing.T) {
	registry := core.NewRegistry()
	server := NewServer(registry, "127.0.0.1", 8080)

	// 测试空 url 参数
	reqEmpty := httptest.NewRequest(http.MethodGet, "/api/proxy", nil)
	wEmpty := httptest.NewRecorder()
	server.handleProxy(wEmpty, reqEmpty)
	if wEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty url, got %d", wEmpty.Code)
	}

	// 测试非 http/https 协议
	reqFtp := httptest.NewRequest(http.MethodGet, "/api/proxy?url=ftp://example.com/1.jpg", nil)
	wFtp := httptest.NewRecorder()
	server.handleProxy(wFtp, reqFtp)
	if wFtp.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-http url, got %d", wFtp.Code)
	}
}

// TestHandleConfig 测试配置的查询与保存生效逻辑
func TestHandleConfig(t *testing.T) {
	registry := core.NewRegistry()
	server := NewServer(registry, "127.0.0.1", 8080)

	// 测试 GET 获取配置
	reqGet := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	wGet := httptest.NewRecorder()
	server.handleConfig(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", wGet.Code)
	}

	// 测试 POST 保存新配置
	payload := `{"douyin_cookie":"test_cookie_123"}`
	reqPost := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(payload))
	reqPost.Header.Set("Content-Type", "application/json")
	wPost := httptest.NewRecorder()
	server.handleConfig(wPost, reqPost)

	if wPost.Code != http.StatusOK {
		t.Fatalf("expected status 200 on post, got %d", wPost.Code)
	}

	body := wPost.Body.String()
	if !strings.Contains(body, "配置已保存并生效") {
		t.Errorf("expected success message, got %s", body)
	}

	// 验证抖音源是否由于配置了 Cookie 而被动态激活
	if _, ok := registry.Get("douyin"); !ok {
		t.Errorf("expected douyin source to be registered after setting cookie")
	}
}

// TestServerShutdown 测试 Web 服务的启动与优雅退出
func TestServerShutdown(t *testing.T) {
	registry := core.NewRegistry()
	server := NewServer(registry, "127.0.0.1", 28080)

	// 启动服务协程
	go func() {
		_ = server.Start()
	}()

	// 稍作等待确保端口已监听
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		t.Errorf("failed to shutdown server: %v", err)
	}
}
