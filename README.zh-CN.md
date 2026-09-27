# YuanLook

**属于你自己的私有服务器监控与管理平台。**

[English](README.md) · [私有模式说明](docs/private-mode.md) · [许可证](LICENSE)

YuanLook 基于 [Pika](https://github.com/pika-monitor/pika) 二次开发。**代码仓库公开，部署后的服务器看板和监控数据必须登录才能访问。**

## 核心差异

- 首页、服务器详情、监控页和管理后台均受服务端登录检查保护。
- 主机列表、标签、历史与实时指标、服务监控等接口强制鉴权；旧数据即使标记为公开也不能匿名读取。
- 密码、OIDC、GitHub 登录成功后建立 HttpOnly 页面会话；管理写操作继续使用 Bearer Token / API Key。
- 新增主机、监控默认私有，后台不再提供“匿名可见”选项。
- 探针连接沿用原有 API Key 认证，不依赖浏览器登录。

具体访问边界、登录引导接口和探针下载例外见 [私有模式说明](docs/private-mode.md)。

## 保留的能力

主机资源监控、HTTP/TCP/ICMP 服务检查、告警通知、DDNS、SSH 登录监控、防篡改和 Linux 资产审计。技术栈为 Go + React/TypeScript + SQLite/PostgreSQL + VictoriaMetrics。

## 从源码构建

**当前尚未发布 YuanLook 镜像。仓库继承的 Compose 文件使用 Pika 官方镜像，不包含本项目的私有模式改动。请先构建当前源码。**

需要 Go 1.26+、Node.js 22+、npm、make，以及可连接的 VictoriaMetrics 服务。

```sh
git clone https://github.com/Selei1983/YuanLook.git
cd YuanLook
git clone https://github.com/pika-monitor/pika-default-theme.git ../pika-default-theme
git -C ../pika-default-theme checkout "$(cat .github/default-theme.ref)"
make build-web
go build -o bin/yuanlook ./cmd/serv
cp config.sqlite.yaml config.yaml
```

修改 `config.yaml` 中的管理员密码哈希、JWT 密钥和 VictoriaMetrics 地址，再启动：

```sh
./bin/yuanlook serve --config config.yaml
```

按需从 `./cmd/agent` 编译探针；安装 UPX 后，也可使用 `make build-agents` 构建上游支持的多平台探针。内置探针下载需要将相应产物放在 `bin/agents`。

为保持兼容，Go 模块路径、探针协议、服务名称及部分界面文字暂时保留 Pika 命名。默认看板主题来自独立上游仓库，版本由 `.github/default-theme.ref` 锁定。

## 验证状态

私有模式的服务端与探针回归测试、前端生产构建和 ESLint 已在本地通过；尚未完成真实服务器部署及第三方 OAuth 提供商联调。

```sh
go test ./internal/... ./pkg/agent/...
npm ci --prefix web
npm run build --prefix web
npm run lint --prefix web
```

继承的上游镜像和 Release 发布流程仅允许在上游仓库运行；YuanLook 使用独立的验证流程。

## 来源与许可

基于 Pika 提交 `f391cb15a60efdc979672baba915cb85e67e8711`，保留原作者版权声明与 Apache-2.0 许可证。YuanLook 是独立衍生项目，不是 Pika 官方发布版本。详见 [UPSTREAM.md](UPSTREAM.md)。
