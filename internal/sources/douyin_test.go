package sources

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/slowlyo/meme/internal/core"
)

// TestNormalizeDouyinCookie 测试抖音 Cookie 各种格式输入的自动规整能力
func TestNormalizeDouyinCookie(t *testing.T) {
	// 测试纯 32 位 hex sessionid 自动补齐为 sessionid=xxx
	rawHex := "996a3db26a82401454f2c8eb1946a815"
	normalized := normalizeDouyinCookie(rawHex)
	expected := "sessionid=996a3db26a82401454f2c8eb1946a815"
	if normalized != expected {
		t.Errorf("normalizeDouyinCookie failed, got %q, want %q", normalized, expected)
	}

	// 测试已经带有 sessionid= 的情况保持原样
	rawWithKey := "sessionid=996a3db26a82401454f2c8eb1946a815"
	if normalizeDouyinCookie(rawWithKey) != rawWithKey {
		t.Errorf("normalizeDouyinCookie failed for already keyed string")
	}

	// 测试从浏览器复制的 cURL 命令中提取 sessionid
	curlCmd := `curl 'https://www.douyin.com/aweme/v1/' -H 'Cookie: passport_csrf_token=abc; sessionid=996a3db26a82401454f2c8eb1946a815; other=123'`
	normalizedCurl := normalizeDouyinCookie(curlCmd)
	if normalizedCurl != expected {
		t.Errorf("normalizeDouyinCookie failed for cURL command, got %q, want %q", normalizedCurl, expected)
	}

	// 测试从同时带有 sessionid 和 ttwid 的完整文本中提取
	fullCookieText := "sessionid=996a3db26a82401454f2c8eb1946a815; ttwid=test_ttwid_value"
	if normalizeDouyinCookie(fullCookieText) != fullCookieText {
		t.Errorf("normalizeDouyinCookie failed for full cookie text")
	}

	// 测试空串边界情况
	if normalizeDouyinCookie("   ") != "" {
		t.Errorf("normalizeDouyinCookie failed for whitespace string")
	}
}

// TestFetchDouyinTTWID 测试通过官方公开端点自动获取 ttwid 凭证
func TestFetchDouyinTTWID(t *testing.T) {
	client := newHTTPClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ttwid, err := fetchDouyinTTWID(ctx, client)
	if err != nil {
		t.Fatalf("fetchDouyinTTWID failed: %v", err)
	}

	if ttwid == "" || !strings.HasPrefix(ttwid, "1%7C") {
		t.Errorf("invalid ttwid format: %s", ttwid)
	}
}

// TestDouyinSourceWithSessionIDOnly 测试仅提供 sessionid 时的完整检索流程
func TestDouyinSourceWithSessionIDOnly(t *testing.T) {
	// 仅提供 32 位纯 sessionid，测试后端自动注入 ttwid 并成功检索
	sessionID := "996a3db26a82401454f2c8eb1946a815"
	source := NewDouyin(sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	memes, err := source.Search(ctx, "猫", core.SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Douyin search failed with sessionid only: %v", err)
	}

	if len(memes) == 0 {
		t.Errorf("expected memes from douyin, got empty")
	}
}
