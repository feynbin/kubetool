APP_NAME := kubetool
IMAGE_NAME := $(APP_NAME):1.0
CONTAINER_NAME := $(APP_NAME)-runner
LOCAL_PROXY := http://host.docker.internal:8081/repository/go/

.PHONY: help build run logs clean stop all

help:
	@echo "Usage:"
	@echo "  make build    # 构建Docker镜像 (使用本地代理)"
	@echo "  make run      # 运行容器并挂载output目录查看产物"
	@echo "  make logs     # 查看容器日志"
	@echo "  make stop     # 停止并删除容器"
	@echo "  make clean    # 停止容器并删除镜像"
	@echo "  make all      # 完整流程: build -> run -> 等待 -> logs -> stop"

# 构建镜像
build:
	@echo "构建镜像中...(使用代理: $(LOCAL_PROXY))"
	docker build \
		--build-arg GOPROXY="$(LOCAL_PROXY),https://goproxy.cn,direct" \
		--progress=plain \
		-t $(IMAGE_NAME) .
	@echo "镜像构建完成: $(IMAGE_NAME)"

# 运行容器
run:
	@echo "启动容器..."
	docker run --rm \
		--name $(CONTAINER_NAME) \
		-v "$$(pwd)/output:/data" \
		$(IMAGE_NAME)
	@echo "容器已启动，名称: $(CONTAINER_NAME)"
	@echo "程序输出目录: $$(pwd)/output"

# 查看日志
logs:
	docker logs -f $(CONTAINER_NAME)

# 停止并删除容器
stop:
	@echo "停止容器..."
	@-docker stop $(CONTAINER_NAME) 2>/dev/null || true
	@-docker rm $(CONTAINER_NAME) 2>/dev/null || true
	@echo "容器已清理"

# 清理镜像
clean: stop
	@echo "删除镜像..."
	@-docker rmi $(IMAGE_NAME) 2>/dev/null || true

# 完整演示流程
all: build run
	@echo "等待程序运行5秒..."
	@sleep 5
	@echo "查看最近日志..."
	@docker logs --tail 20 $(CONTAINER_NAME)
	@echo "------"
	@echo "流程结束。如需持续查看日志，请执行: make logs"
	@echo "如需停止，请执行: make stop"

# 检查环境
env-check:
	@echo "检查环境:"
	@echo "1. OS: $$(uname -a)"
	@echo "2. make版本: $$(make --version | head -1)"
	@echo "3. docker版本: $$(docker --version)"
	@echo "4. 当前目录: $$(pwd)"
	@echo "5. 输出目录是否存在: $$(if [ -d "output" ]; then echo "是";  echo "否"; fi)"