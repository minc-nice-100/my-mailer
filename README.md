# My Mailer Simple

这是一个简易版本的邮件发送服务，使用内存队列，不依赖数据库。

## 环境变量

- SMTP_HOST SMTP服务器地址  
- SMTP_PORT SMTP端口  
- SMTP_USER SMTP用户名  
- SMTP_PASS SMTP密码  

## 使用示例

POST /send 接口接收 XML 格式邮件请求，示例请求体：

```xml
<mail>
  <to>user@example.com</to>
  <subject>验证码</subject>
  <body>你的验证码是123456</body>
</mail>
```

curl 示例：

```bash
curl -X POST http://localhost:8080/send   -H "Content-Type: application/xml"   -d '<mail><to>user@example.com</to><subject>验证码</subject><body>你的验证码是123456</body></mail>'
```

## 启动

```bash
docker-compose up -d
```

## 许可证

MIT
