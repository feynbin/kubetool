FROM golang:1.25.6-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/kubetool .

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/feynbin/kubetool"
COPY --from=builder /app/kubetool .
ENTRYPOINT ["/kubetool"]