# 多階段構建
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 複製依賴文件
COPY go.mod go.sum ./
RUN go mod download

# 複製源碼
COPY . .

# 構建二進制文件
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o greeter-service .

# 最終鏡像
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# 從 builder 複製二進制文件
COPY --from=builder /app/greeter-service .

# 暴露端口
EXPOSE 9090

# 運行服務
CMD ["./greeter-service"]
