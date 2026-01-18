FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/kubetool .

# 运行阶段
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
# 从构建阶段复制二进制文件
COPY --from=builder /app/kubetool .
# 创建数据目录
RUN mkdir -p /data
# 设置数据卷，方便检查产物
VOLUME /data
CMD ["./kubetool"]