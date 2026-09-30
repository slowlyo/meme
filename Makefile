.PHONY: build run test clean deps install

# 二进制名称
APP_NAME := web
BUILD_DIR := ./build

# 构建 Web 服务
build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) .

# 直接运行服务
run:
	go run .

# 跨平台构建
build-release:
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .

# 单元测试
test:
	go test -v ./...

# 清理
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

# 安装依赖
deps:
	go mod tidy
	go mod download

# 安装到系统
install: build
	@echo "Installing to /usr/local/bin..."
	@cp $(BUILD_DIR)/$(APP_NAME) /usr/local/bin/

# 格式化代码
fmt:
	go fmt ./...

# 代码检查
lint:
	golangci-lint run
