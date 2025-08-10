# My Mailer - Go 语言自建邮件验证码发送服务

## 功能

- HTTP POST `/send` 接收 XML 邮件请求  
- 使用 DuckDB 做持久化队列  
- 多 worker 并发消费邮件任务  
- SMTP 配置通过环境变量传入  
- 失败任务自动重试，支持最大3次重试  
- 支持 Docker 容器化和 Docker Compose 一键部署  
- GitHub Actions 自动构建推送镜像到 GHCR  

## 环境变量

- `SMTP_HOST` SMTP服务器地址  
- `SMTP_PORT` SMTP端口  
- `SMTP_USER` SMTP用户名  
- `SMTP_PASS` SMTP密码  

## API 使用示例

请求方式：POST  
Content-Type: `application/xml`  

请求体示例：

```xml
<mail>
  <to>user@example.com</to>
  <subject>验证码</subject>
  <body>你的验证码是123456</body>
</mail>
```

示例 curl：

```bash
curl -X POST http://localhost:8080/send   -H "Content-Type: application/xml"   -d '<mail><to>user@example.com</to><subject>验证码</subject><body>你的验证码是123456</body></mail>'
```

## 部署

1. 克隆本仓库：

```bash
git clone https://github.com/<你的用户名>/my-mailer.git
cd my-mailer
```

2. 修改 `docker-compose.yml` 中的 SMTP 配置和镜像地址。

3. 启动服务：

```bash
docker-compose up -d
```

## CI/CD

推送代码到 `main` 分支，GitHub Actions 会自动构建并推送镜像到 GitHub Container Registry。

## 许可证

MIT
