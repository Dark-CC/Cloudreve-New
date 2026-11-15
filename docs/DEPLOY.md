# 部署

## 单机二进制

```bash
./cloudreve -c data/conf.ini server
```

- 首次启动自动生成 `data/conf.ini`、数据库文件，并在控制台打印管理员账号；
- 默认监听 `:5212`，可用 `[System] Listen` 修改；
- 生产环境建议用 systemd 托管，`Restart=always`，并用 Nginx 反向代理。

### systemd 示例

```ini
[Unit]
Description=Cloudreve-New
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/Cloudreve-New
ExecStart=/opt/Cloudreve-New/cloudreve -c data/conf.ini server
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

## Docker Compose

```bash
docker compose up -d
```

`src/docker-compose.yml` 内含 PostgreSQL 与 Redis，需把镜像名改为你发布的镜像。

## 数据库

支持 SQLite（默认）、PostgreSQL、MySQL。切换数据库时修改 `conf.ini` 的
`[Database]` 段，或使用 `CR_CONF_Database.*` 环境变量。

## 反向代理要点

- 需要透传 `Host`、`X-Forwarded-For`、`X-Forwarded-Proto`；
- 上传大文件时放开 `client_max_body_size`；
- 管理端"站点 URL"必须与浏览器实际访问的地址一致，否则部分回调会失败。

## 静态资源

静态资源已通过 `//go:embed` 打进二进制，不需要单独部署 `web/`。
替换前端后必须重新编译后端，或删除旧的 `statics` 缓存。
