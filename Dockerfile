FROM golang:1.22-alpine AS builder
WORKDIR /app

# 只复制 go.mod，避免找不到 go.sum 错误
COPY go.mod ./

RUN go mod download

COPY . .

RUN go build -o mailer main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/mailer .

EXPOSE 8080

ENTRYPOINT ["./mailer"]
