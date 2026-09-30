# 阶段 1：构建应用二进制
FROM golang:1.23-alpine AS builder

WORKDIR /src

# 安装基础证书与时区定义
RUN apk add --no-cache ca-certificates tzdata

# 先复制依赖定义，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制完整源码
COPY . .

# 编译纯静态无依赖二进制
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/web .

# 阶段 2：运行最小环境
FROM alpine:latest

WORKDIR /app

# 从构建器中复制根证书、时区与可执行文件
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /app/web /app/web

# 默认环境变量
ENV PORT=23333
ENV TZ=Asia/Shanghai

# 暴露不常用的表情包端口
EXPOSE 23333

# 容器启动命令：监听所有网络接口，不唤起宿主机桌面浏览器
ENTRYPOINT ["/app/web", "-host", "0.0.0.0", "-no-browser"]
