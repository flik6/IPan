# iPan（Go + Vue 版）

联通云盘（沃盘）的 Web 文件管理端。**Go 后端 + Vue 3 前端 + 内置 SQLite**，单个 Docker 容器即可部署。

这是原 [Cloudflare Workers 版本](../wopan-cf-worker) 的重写版本：

- **无需 KV / R2 绑定**：所有状态（访问密码、云盘令牌、平台分享、Passkey 凭证、会话）都存在容器内的 SQLite 文件里。
- **无需 Wrangler / Cloudflare 账号**：任何能跑 Docker 的机器都能部署，也可以直接跑二进制文件。
- **接口与前端完全兼容**：`/api/*` 的路径、请求体与响应结构与 Worker 版一致，Vue 前端从原项目迁移而来，只改了构建方式与少量文案。
- **更适合大文件与长连接**：流式上传/下载、Range 断点续传、分片并发都由 Go 的原生 HTTP 处理，没有 Worker 的 CPU/内存限制。

## 功能

- **文件管理**：列目录、分页、新建目录、重命名、移动、复制、删除
- **上传**：浏览器端 8MB 分片上传（`/api/upload/init` + `/api/upload/part`）、服务端流式上传（`PUT /api/upload`）、multipart 多文件上传
- **下载**：302 跳转直链、流式预览（`/api/view`，支持 Range）、流式下载（`/api/proxy-download`）、zip 打包下载
- **文件床**：`/api/bed?fid=&sign=`，签名 `md5(fid + SIGN_PARAM)`，可生成稳定的公开直链
- **分享**：官方分享（联通云盘链接 + 提取码）、平台分享（本服务 `/s/<id>` + 提取码 + 有效期）、直链
- **分享中心**：集中查看/取消官方分享与平台分享
- **直链解析**：粘贴沃盘分享链接（含短链）自动解析出下载地址
- **云盘登录**：手机号 + 短信验证码在线登录
- **令牌保活**：后台定时器按 2 天间隔自动刷新令牌（滑动过期），长期不掉登录
- **访问控制**：访问密码 + Passkey（WebAuthn）免密登录，会话 180 天
- **界面**：Vue 3 + naive-ui，浅色/深色/跟随系统，桌面与移动端自适应

## 快速开始

### Docker Compose（推荐）

```bash
cp .env.example .env      # 可选：改端口、设置预置密码等
docker compose up -d --build
```

打开 `http://localhost:8080`，首次访问会引导设置访问密码，然后用手机号 + 短信验证码绑定云盘账号。

数据保存在名为 `ipan-data` 的 Docker 卷里（挂载到容器的 `/app/data`），重建容器不会丢失。

### Docker（不使用 compose）

```bash
docker build -t ipan:latest .
docker run -d --name ipan -p 8080:8080 -v ipan-data:/app/data \
  -e TZ=Asia/Shanghai ipan:latest
```

### 本地运行（开发）

需要 Go 1.26+ 与 Node 20+。

```bash
# 1) 构建前端
cd web && npm install && npm run build && cd ..

# 2) 运行后端（前端产物由 Go 直接托管）
go run ./cmd/ipan -listen :8080 -data ./data -static ./web/dist
```

前端热更新开发时，另开一个终端跑 `npm run dev`（在 `web/` 目录），Vite 会把 `/api` 代理到 `http://127.0.0.1:8080`。

## 配置

所有配置都通过环境变量传入。首次启动后，访问密码、签名密钥、云盘令牌会写入数据库并优先生效；
环境变量只作为「库里没有记录时」的初始值。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `IPAN_PORT` | `8080` | docker compose 映射到宿主机的端口 |
| `IPAN_LISTEN` | `:8080` | 容器内监听地址（直接跑二进制时使用） |
| `IPAN_DATA_DIR` | `./data` | 数据目录，SQLite 文件为 `ipan.db` |
| `IPAN_PUBLIC_URL` | 自动推断 | 生成分享链接时使用的站点地址，反向代理场景建议显式设置 |
| `IPAN_TRUST_PROXY` | `false` | 信任 `X-Forwarded-*`。放在 Nginx/Traefik 后面时**必须**打开，否则 Passkey 拿不到正确域名 |
| `APP_TOKEN` | 空 | 预置访问密码。留空则首次访问时引导设置 |
| `SIGN_PARAM` | 空 | 文件床签名密钥。留空则自动随机生成并持久化 |
| `WOPAN_ACCESS_TOKEN` | 空 | 预置云盘 access token。也可在页面登录后自动写入 |
| `WOPAN_REFRESH_TOKEN` | 空 | 预置 refresh token，用于自动保活 |
| `WOPAN_BASE_URL` | `https://panservice.mail.wo.cn` | 云盘接口域名 |
| `WOPAN_ZONE_URL` | 动态发现 | 上传域名；留空时通过 `GetZoneInfo` 自动发现 |
| `WOPAN_SPACE_TYPE` | `0` | `0`=个人空间，`1`=家庭空间 |
| `WOPAN_FAMILY_ID` | 空 | 家庭空间 ID（`WOPAN_SPACE_TYPE=1` 时必填） |
| `TOKEN_CHECK_INTERVAL_HOURS` | `1` | 后台检查令牌的间隔；设为 `0` 关闭自动保活 |
| `IPAN_ENABLE_PASSKEY` | `true` | 是否启用 Passkey 登录 |

## 反向代理示例

Passkey 绑定域名（WebAuthn 要求 RP ID 是域名，不能用 IP），所以生产环境建议挂在域名后面。

```nginx
server {
    listen 443 ssl;
    server_name pan.example.com;

    client_max_body_size 0;      # 上传不限制体积

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_request_buffering off;   # 大文件边收边转
        proxy_buffering off;
    }
}
```

对应的 `.env`：

```
IPAN_PORT=8080
IPAN_PUBLIC_URL=https://pan.example.com
IPAN_TRUST_PROXY=true
```

## 项目结构

```
cmd/ipan/           程序入口：读配置、启动 HTTP 服务、后台令牌保活
internal/config/    环境变量解析
internal/store/     SQLite 持久化（设置、令牌、分享、Passkey、会话、缓存）
internal/wopan/     联通云盘 API 客户端（AES-CBC 加密、请求签名、令牌刷新、分片上传）
internal/httpapi/   HTTP 路由与处理函数（鉴权、文件、下载、分享、用户、Passkey、静态资源）
web/                Vue 3 前端（由 Go 直接托管构建产物）
```

## 接口

与原 Worker 版本保持一致，除特别说明外都需要 `Authorization: Bearer <访问密码>`。

### 公开接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/setup/status` | 是否已初始化 |
| POST | `/api/setup` | 设置访问密码，body `{ password, signParam? }` |
| GET | `/api/bed?fid=&sign=[&name=&mode=proxy]` | 文件床，`sign=md5(fid+SIGN_PARAM)` |
| GET | `/api/public/share?id=` | 平台分享元信息 |
| POST | `/api/public/share/verify` | 校验提取码，body `{ id, code }` |
| GET | `/api/public/share/download?id=&code=` | 单文件直链 / 多项目 zip |
| GET | `/api/public/share/file?id=&code=&fid=` | 分享内单文件直链 |
| POST | `/api/passkey/auth/options` \| `/auth/verify` | Passkey 登录 |

### 需要登录

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/auth-check` | 鉴权 + 云盘登录态（首屏一次往返） |
| GET | `/api/login/status` | 云盘绑定与自动保活状态 |
| POST | `/api/login/send-code` | 发送短信验证码 |
| POST | `/api/login/verify` | 校验验证码并保存云盘令牌 |
| POST | `/api/logout` | 清除会话 Cookie |
| GET/POST | `/api/settings` · `/settings/show-hidden` · `/settings/password` | 应用设置 |
| GET | `/api/list?parentId=&page=&pageSize=` | 列目录 |
| POST | `/api/mkdir` · `/rename` · `/delete` · `/move` · `/copy` | 文件操作 |
| POST | `/api/upload/init` · `/upload/part` | 浏览器分片上传 |
| PUT/POST | `/api/upload?directoryId=&name=` | 流式 / multipart 上传 |
| GET | `/api/download` · `/view` · `/proxy-download` · `/direct` | 下载与预览 |
| POST | `/api/direct` | 批量直链 + 文件床链接 |
| POST | `/api/pack-url` · `/download-zip` | zip 打包 |
| POST | `/api/share` · `/api/shares/*` | 创建/管理分享 |
| POST | `/api/resolve` | 解析沃盘分享链接 |
| GET | `/api/detail` · `/api/user/profile` | 账号信息 |
| POST | `/api/user/profile` · `/api/user/avatar` | 修改资料 / 上传头像 |
| POST | `/api/refresh` | 手动刷新云盘令牌 |

## 测试

```bash
go test ./...                 # 单元测试 + HTTP 接口测试
cd web && npm run typecheck   # 前端类型检查
```

线上接口连通性测试（会真实访问联通云盘，默认跳过）：

```bash
IPAN_LIVE_TEST=1 WOPAN_ACCESS_TOKEN=... go test ./internal/wopan -run Live -v
```

## 数据与备份

全部状态都在 `IPAN_DATA_DIR/ipan.db`（SQLite，WAL 模式）。备份只要拷贝这个文件即可：

```bash
docker compose exec ipan sh -c 'ls -l /app/data'
docker cp ipan:/app/data/ipan.db ./ipan-backup.db
```

## 已知限制

- 联通云盘接口按 `fid` / `directoryId` 操作，不支持 `/a/b.txt` 路径寻址，必须先列目录拿 ID。
- Passkey 需要域名访问；用 `IP 地址:端口` 打开时会提示改用域名。
- 数据库采用单连接串行写入，适合个人/小团队使用，不是多租户设计。
- 原 Worker 版的 KV 数据不会自动迁移，首次启动需要重新设置访问密码并绑定云盘。
