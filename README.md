# iPan（Go + Vue 版）

联通云盘（沃盘）的 Web 文件管理端。**Go 后端 + Vue 3 前端 + 内置 SQLite**，单个 Docker 容器即可部署。

- **单容器部署**：前端、Go 服务和 SQLite 都打包在一个镜像里，也可以直接运行二进制文件。
- **本地持久化**：访问密码、云盘令牌、分享、Passkey、Session 和 WebDAV 凭据都保存在 `ipan.db`。
- **大文件友好**：流式上传/下载、Range 断点续传与分片上传由 Go HTTP 服务直接处理。
- **后台配置**：首次访问设置访问密码，之后在网页内绑定云盘、配置 WebDAV 和显示偏好。

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
- **WebDAV**：通过 `/dav/` 挂载已绑定的云盘空间，支持读写、文件锁和自定义属性
- **界面**：Vue 3 + naive-ui，浅色/深色/跟随系统，桌面与移动端自适应

## 快速开始

### Docker Compose（推荐）

```bash
docker compose up -d
```

打开 `http://localhost:8080`，首次访问会引导设置访问密码，然后用手机号 + 短信验证码绑定云盘账号。

数据保存在名为 `ipan-data` 的 Docker 卷里（挂载到容器的 `/app/data`），重建容器不会丢失。
普通使用不需要 `.env` 或配置文件，访问密码、云盘绑定、WebDAV 凭据都在网页后台设置。

如果启动后出现 `unable to open database file (14)`，通常是数据目录权限问题。新版镜像会自动修正
named volume 的权限后再降权运行；使用 NAS bind mount 时，请确保宿主机目录允许 UID `100`、GID `101`
写入，或在宿主机执行一次 `chown -R 100:101 /你的数据目录`。

### Docker（不使用 compose）

```bash
docker build -t ipan:latest .
docker run -d --name ipan -p 8080:8080 -v ipan-data:/app/data \
  -e TZ=Asia/Shanghai ipan:latest
```

### GitHub Actions 自动构建

推送到 `master`/`main` 或创建 `v*` 标签时，GitHub Actions 会自动构建并发布镜像到 GitHub Container Registry：

```bash
docker pull ghcr.io/flik6/ipan:latest
docker run -d --name ipan -p 8080:8080 -v ipan-data:/app/data \
  -e TZ=Asia/Shanghai ghcr.io/flik6/ipan:latest
```

镜像发布使用工作流内置的 `GITHUB_TOKEN`，不需要额外配置 Secret。若仓库的 Actions 默认权限被限制为只读，请在仓库 **Settings → Actions → General → Workflow permissions** 中选择 **Read and write permissions**。

### 本地运行（开发）

需要 Go 1.26+ 与 Node 20+。

```bash
# 1) 构建前端
cd web && npm install && npm run build && cd ..

# 2) 运行后端（前端产物由 Go 直接托管）
go run ./cmd/ipan -listen :8080 -data ./data -static ./web/dist
```

前端热更新开发时，另开一个终端跑 `npm run dev`（在 `web/` 目录），Vite 会把 `/api` 代理到 `http://127.0.0.1:8080`。

## 高级配置

正常使用不需要环境变量。访问密码、签名密钥、云盘令牌、Passkey 和 WebDAV 凭据都会写入 SQLite；
以下环境变量只用于自动化部署、反向代理或高级调试。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `IPAN_LISTEN` | `:8080` | 容器内监听地址（直接跑二进制时使用） |
| `IPAN_DATA_DIR` | `./data` | 数据目录，SQLite 文件为 `ipan.db` |
| `IPAN_PUBLIC_URL` | 自动推断 | 生成分享链接时使用的站点地址，反向代理场景建议显式设置 |
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

无需额外环境变量。iPan 会自动读取 `Host`、`X-Forwarded-Proto` 和 `X-Forwarded-Host`；
只有代理没有传递这些头时，才需要通过 `IPAN_PUBLIC_URL` 指定外部地址。

## WebDAV 挂载

先绑定联通云盘账号，再进入 **设置 → WebDAV**，设置独立用户名和密码并保存，立即生效，无需重启。
用户名不能包含冒号；关闭 WebDAV 后 `/dav/` 返回 404。凭据保存在 SQLite，不保存明文密码。
WebDAV 客户端填写 `https://你的域名/dav/`，同时支持 Basic 与 Digest 认证。
HTTP 连接也能使用 Basic，但密码会以明文传输，建议通过 HTTPS 挂载。
rclone 的存储类型选 `webdav`，供应商选 `other`。
在反向代理后挂载时，转发 `X-Forwarded-Proto` 与 `X-Forwarded-Host` 即可；也可将 `IPAN_PUBLIC_URL` 设为客户端使用的外部地址，确保 COPY 和 MOVE 的绝对目标地址能通过校验。 [RFC 7616](https://www.rfc-editor.org/rfc/rfc7616)

支持 `OPTIONS`、`GET`、`HEAD`、`PUT`、`DELETE`、`MKCOL`、`COPY`、`MOVE`、`PROPFIND`、`PROPPATCH`、`LOCK` 和 `UNLOCK`。
接口按 RFC 4918 实现。目录枚举支持 `Depth: 0` 和 `Depth: 1`，省略 Depth 或请求 `Depth: infinity` 时返回 403 和 `propfind-finite-depth`，客户端可逐层枚举。
目录 COPY 支持 `Depth: 0` 和 `Depth: infinity`，目录 MOVE 和 DELETE 处理整个子树。 [RFC 4918](https://www.rfc-editor.org/rfc/rfc4918)

`OPTIONS`、`PROPFIND`、`PROPPATCH`、`MKCOL` 和 `DELETE` 使用 Go 的 `x/net/webdav` 处理器。
iPan 的适配层负责把路径转换为云盘 ID，并处理云端流式传输、原生复制与移动、写锁和属性持久化。 [Go WebDAV 包文档](https://pkg.go.dev/golang.org/x/net@v0.62.0/webdav)

- 自定义属性保存在 SQLite。写锁在当前 iPan 进程的 WebDAV 请求之间生效，重启即失效。联通官方客户端和 iPan 网页可以独立修改云端文件。
- 已知长度的 PUT 按 8 MB 分片上传。未知长度的请求先写入系统临时目录，以取得上游所需的文件长度。
- 覆盖上传、复制和部分移动会创建以 `.ipan-dav-` 开头的临时条目。iPan 会尝试回滚失败的替换。进程中断或云端拒绝清理时，可能留下用于恢复的临时条目。

### 与联通云盘桌面版对照

下表来自本地 Windows 客户端 2.8.13 安装包和日志的静态检查。桌面端能力的实际请求和响应未做联机验证。

| 能力 | 联通云盘桌面版 | iPan |
| --- | --- | --- |
| 列目录、建目录、改名 | `QueryAllFiles`、`CreateDirectory`、`RenameFileOrDirectory` | WebDAV 复用现有云盘调用，逐段将路径解析为 ID |
| 下载、分片上传 | `GetDownloadUrlV2`、`/openapi/client/upload2C` | WebDAV 复用直链和分片上传，提供 GET、HEAD、Range 和 PUT |
| 批量文件操作 | `/wohome/pass/file/batch/createTask` | WebDAV 复用 iPan 已有的 `DeleteFile`、`MoveFile`、`CopyFile` |
| 属性、条件请求、锁 | 安装包的业务脚本和日志中未发现 DAV 入口 | 由 iPan 的 `/dav/` 实现 |
| 搜索、回收站、文件版本 | `/wohome/pc/queryFilesByName`、`QueryRecycleData`、`/wohome/fileVersionDispatcher` | 目前无对应接口，属于 RFC 4918 之外的桌面功能 |

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

除特别说明外，接口都需要 `Authorization: Bearer <访问密码>`。

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
| GET/POST | `/api/settings` · `/settings/show-hidden` · `/settings/password` · `/settings/webdav` | 应用设置；WebDAV 支持 GET/POST/DELETE |
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
