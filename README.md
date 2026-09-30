# 表情包聚合检索工具

基于 Go 语言开发的高性能多源表情包检索 Web 服务。

## 特性

- **高性能并发**：支持多源并发检索与 SSE 流式增量加载。
- **现代化 Web 界面**：自适应暗色模式、单键复制图片（可直接在微信/飞书/QQ中粘贴发送）、双击新标签页预览。
- **免繁琐配置**：内置智能防盗链图片代理；抖音源自动补齐设备凭证（ttwid）。
- **极小 Docker 镜像**：多阶段构建，镜像大小仅约 15MB，默认使用不常用端口 `23333`。
- **智能去重**：自动清洗并过滤跨平台重复图片。

## 支持的数据源

| ID | 名称 | 站点 | 配置要求 |
|:---|:-----|:-----|:---------|
| `doutula` | 斗图啦 | doutupk.com | 免配置开箱即用 |
| `sougou` | 搜狗表情 | pic.sogou.com | 免配置开箱即用 |
| `pdan` | 胖哒 | pdan.com.cn | 免配置开箱即用 |
| `douyin` | 抖音 | douyin.com | 需配置 sessionid / cURL |

## 快速开始

### 方式一：Docker 运行（推荐）

#### 1. 直接运行容器

```bash
docker run -d \
  --name doutu-web \
  --restart unless-stopped \
  -p 23333:23333 \
  -v $(pwd)/.config.json:/app/.config.json \
  ghcr.io/slowlyo/meme:latest
```

#### 2. Docker Compose

```yaml
version: '3.8'

services:
  doutu-web:
    image: ghcr.io/slowlyo/meme:latest
    container_name: doutu-web
    restart: unless-stopped
    ports:
      - "23333:23333"
    volumes:
      - ./.config.json:/app/.config.json
```

服务就绪后访问：`http://localhost:23333`

---

### 方式二：本地运行

```bash
# 直接运行（默认监听 23333 端口并自动唤起浏览器）
make run

# 编译为二进制
make build
./build/web -port 23333
```

## 配置说明

配置文件默认保存于当前目录下的 `.config.json`（支持热重载，无需重启服务）：

```json
{
  "douyin_cookie": "your_sessionid_or_curl"
}
```

### 抖音凭证获取方式（二选一）

- **直接粘贴 cURL（推荐）：** 登录网页版 `douyin.com`，F12 ➔ Network(网络) 刷新 ➔ 随便右键一个请求 ➔ **Copy as cURL** ➔ 直接全量粘贴至 Web 设置框（系统已支持正则全自动提取，无需手动挑拣）。
- **Application 面板：** 登录 `douyin.com`，F12 ➔ Application(应用程序) ➔ Cookies ➔ 找到 `sessionid` 复制其 32 位值。

## 许可证

MIT
