package core

import (
	"context"
	"sync"
	"time"
)

// Registry 源注册中心
type Registry struct {
	sources map[string]Source
	mu      sync.RWMutex
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[string]Source),
	}
}

// Register 注册一个源
func (r *Registry) Register(source Source) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[source.ID()] = source
}

// Unregister 根据标识从注册中心注销指定源
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sources, id)
}

// Get 获取指定源
func (r *Registry) Get(id string) (Source, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, ok := r.sources[id]
	return source, ok
}

// List 列出所有源
func (r *Registry) List() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sources := make([]Source, 0, len(r.sources))
	for _, s := range r.sources {
		sources = append(sources, s)
	}
	return sources
}

// ListIDs 列出所有源ID
func (r *Registry) ListIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sources))
	for id := range r.sources {
		ids = append(ids, id)
	}
	return ids
}

// SearchAll 并发搜索所有源
func (r *Registry) SearchAll(ctx context.Context, keyword string, opts SearchOptions) SearchResult {
	startTime := time.Now()

	sources := r.List()
	if len(sources) == 0 {
		return SearchResult{
			Memes:      []Meme{},
			Sources:    []string{},
			Errors:     map[string]string{},
			Total:      0,
			DurationMs: time.Since(startTime).Milliseconds(),
		}
	}

	// 创建结果通道
	type sourceResult struct {
		sourceID string
		memes    []Meme
		err      error
	}

	resultCh := make(chan sourceResult, len(sources))

	// 并发请求所有源
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()

			// 为每个源创建带超时的 context
			timeout := opts.Timeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}
			sourceCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			memes, err := s.Search(sourceCtx, keyword, opts)
			resultCh <- sourceResult{
				sourceID: s.ID(),
				memes:    memes,
				err:      err,
			}
		}(source)
	}

	// 等待所有请求完成后关闭通道
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	var allMemes []Meme
	successSources := []string{}
	errors := make(map[string]string)

	for result := range resultCh {
		if result.err != nil {
			errors[result.sourceID] = result.err.Error()
		} else {
			successSources = append(successSources, result.sourceID)
			allMemes = append(allMemes, result.memes...)
		}
	}

	// 去重
	allMemes = DeduplicateMemes(allMemes)

	return SearchResult{
		Memes:      allMemes,
		Sources:    successSources,
		Errors:     errors,
		Total:      len(allMemes),
		DurationMs: time.Since(startTime).Milliseconds(),
	}
}

// SearchSources 搜索指定的源
func (r *Registry) SearchSources(ctx context.Context, keyword string, sourceIDs []string, opts SearchOptions) SearchResult {
	startTime := time.Now()

	if len(sourceIDs) == 0 {
		return r.SearchAll(ctx, keyword, opts)
	}

	// 创建结果通道
	type sourceResult struct {
		sourceID string
		memes    []Meme
		err      error
	}

	resultCh := make(chan sourceResult, len(sourceIDs))

	// 并发请求指定源
	var wg sync.WaitGroup
	for _, id := range sourceIDs {
		source, ok := r.Get(id)
		if !ok {
			resultCh <- sourceResult{
				sourceID: id,
				err:      ErrSourceNotFound,
			}
			continue
		}

		wg.Add(1)
		go func(s Source) {
			defer wg.Done()

			timeout := opts.Timeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}
			sourceCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			memes, err := s.Search(sourceCtx, keyword, opts)
			resultCh <- sourceResult{
				sourceID: s.ID(),
				memes:    memes,
				err:      err,
			}
		}(source)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	var allMemes []Meme
	successSources := []string{}
	errors := make(map[string]string)

	for result := range resultCh {
		if result.err != nil {
			errors[result.sourceID] = result.err.Error()
		} else {
			successSources = append(successSources, result.sourceID)
			allMemes = append(allMemes, result.memes...)
		}
	}

	allMemes = DeduplicateMemes(allMemes)

	return SearchResult{
		Memes:      allMemes,
		Sources:    successSources,
		Errors:     errors,
		Total:      len(allMemes),
		DurationMs: time.Since(startTime).Milliseconds(),
	}
}

// 全局默认注册中心
var DefaultRegistry = NewRegistry()
