package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/slowlyo/meme/internal/core"
	"github.com/slowlyo/meme/internal/sources"
	"github.com/slowlyo/meme/internal/web"
)

// openBrowser 在系统默认浏览器中打开指定 URL
func openBrowser(url string) {
	var cmd *exec.Cmd
	// 根据不同操作系统选择合适的打开指令
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		// 针对 linux 等其他系统优先使用 xdg-open
		cmd = exec.Command("xdg-open", url)
	}

	// 异步尝试执行打开指令，忽略执行错误避免阻塞服务启动
	_ = cmd.Start()
}

func main() {
	// 定义命令行配置项
	port := flag.Int("port", 8080, "Web 服务监听端口")
	host := flag.String("host", "0.0.0.0", "Web 服务监听地址")
	noBrowser := flag.Bool("no-browser", false, "启动后不自动在浏览器中打开")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `表情包聚合检索 Web 服务

用法:
  web [选项]

选项:
`)
		flag.PrintDefaults()
	}

	flag.Parse()

	// 初始化源注册中心
	registry := core.NewRegistry()

	// 读取配置（整合磁盘配置文件和环境变量）
	config := sources.LoadConfig()

	// 注册全部支持的表情包源
	sources.RegisterAllSources(registry, config)

	// 创建 Web 服务实例
	server := web.NewServer(registry, *host, *port)

	// 监听系统信号以实现优雅退出
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	// 启动 HTTP 服务
	go func() {
		localURL := fmt.Sprintf("http://localhost:%d", *port)
		fmt.Println("-------------------------------------------")
		fmt.Printf("表情包 Web 服务已启动: %s\n", localURL)
		fmt.Printf("监听地址: %s:%d\n", *host, *port)
		fmt.Println("按 Ctrl+C 可停止服务")
		fmt.Println("-------------------------------------------")

		// 如果未禁用自动打开，延迟 150ms 等待监听就绪后唤起浏览器
		if !*noBrowser {
			time.AfterFunc(150*time.Millisecond, func() {
				openBrowser(localURL)
			})
		}

		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "服务异常退出: %v\n", err)
			os.Exit(1)
		}
	}()

	// 阻塞等待退出信号
	<-stopCh
	fmt.Println("\n正在关闭 Web 服务...")

	// 设定 5 秒优雅停机超时
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "关闭服务发生错误: %v\n", err)
	} else {
		fmt.Println("服务已安全退出")
	}
}
