# 多账号私有工作区

## 数据归属与升级

每个密码账号对应一个独立工作区。`App.Users` 中的账号首次启动时导入原数据库的 `platform_accounts` 表；以后密码与启停状态以数据库为准，重启不会覆盖网页上的修改。浏览器使用已验证的 JWT/Cookie 选择工作区；管理 API Key、探针注册密钥和下载密钥按实际所在数据库选择工作区，客户端不能用用户 ID、Header 或 URL 参数切换归属。

原 `database.sqlite.path` 数据库、原主题目录和原有无 `yuanlook_workspace` 标签的指标固定归属于 `App.Workspaces.LegacyOwner`（默认 `admin`）。原来的服务器 ID、监控 ID、密钥和数据保留，无需重装探针。首次启动生成 `<原数据库路径>.workspace-owner`，防止后续误改归属；备份时必须一起保留。

其他账号的数据存储在 `App.Workspaces.Dir/<用户名 SHA256>/pika.db`，主题目录位于同级 `themes`。每个账号使用独立服务实例、缓存、WebSocket 管理器和定时任务。配置文件、JWT 签名密钥、默认主题程序文件及指标数据库服务本身由部署管理员统一维护。

旧指标不改写；新账号指标强制写入其 `yuanlook_workspace` 标签。包括聚合查询、标签枚举、按主机删除和清理孤立数据在内的读写操作均限定工作区。应用只支持 VictoriaMetrics，不能用忽略 `extra_filters[]` 的兼容 API 替代；不得开启 VM 的 `-search.ignoreExtraFiltersAtLabelsAPI`。

## 添加账号

开放注册入口为 `/admin/register`，登录页提供链接。注册后立即启用独立的空白工作区，不需要管理员审核或重启服务。默认开放注册；设 `App.Workspaces.RegistrationEnabled: false` 可关闭注册。

- 用户名为 3–32 位字母、数字、`_`、`-`，首位为字母或数字；用户名是永久身份，不支持重命名或删除后复用。
- 新密码长度为 10–72 字节，以 bcrypt 保存；旧管理员密码继续有效。
- 原数据所有者（默认 `admin`）拥有“账号管理”入口，可查看账号、创建账号、停用/启用账号和重置密码。此权限不允许读取其他账号的监控数据。
- 所有用户可以在右上角个人菜单中修改自己的密码，需提供当前密码。
- 停用或重置密码、自己改密码后，旧 JWT/Cookie 立即失效；重新启用不会恢复旧会话。停用禁止登录、API 调用及新的探针认证，不删除数据，也不暂停已运行的监控任务或主动断开已有探针连接。
- 账号管理和改密码接口只接受用户 Bearer JWT，不接受 Cookie 单独写操作或监控 API Key。注册、登录和密码操作有按 IP 的频率限制。

新增账号无须再写入 `App.Users`。该配置仅用于导入缺失的账号，不会覆盖现有账号；从配置中移除账号也不会停用它，请使用网页账号管理。保留原拥有者的引导配置及归属标记。

部署配置结构：

```yaml
App:
  Users:
    admin: '<已有管理员 bcrypt 哈希>'
  Workspaces:
    LegacyOwner: admin
    RegistrationEnabled: true
    Dir: ./data/workspaces
  VictoriaMetrics:
    Enabled: true
    URL: http://victoriametrics:8428
    Username: yuanlook-storage
    Password: '<随机生成的存储服务密码>'
```

VM 容器需添加 `-httpAuth.username=yuanlook-storage`、`-httpAuth.password=file:///run/secrets/vm-password`，把密码文件只读挂载到该路径，文件内容与应用配置一致。保持 VM 仅在内部网络可达。开放注册或存在多账号时，启动会检查未认证查询返回 401、应用凭据查询成功；检查不通过则拒绝启动。此保护还防止用户可配置的 Webhook 绕过应用直接操作内部指标库。

账号记录、密码哈希和会话版本号与原数据库一起持久保存；备份必须包含原数据库和所有工作区目录。退出登录沿用原 JWT 有效期语义，复制出的 Token 不会因退出立即失效。

## 兼容范围

当前支持 SQLite + 密码账号。OIDC、GitHub OAuth、非 SQLite 数据库会明确拒绝启动，后续需要独立设计身份与空间绑定，不能直接打开原来的 OAuth 开关。`admin` 管理平台账号，同时仍只能查看自己的监控数据。

本项目是自托管工具，支持公开注册，但不提供计费或每用户 CPU/内存配额。主机管理员能读取所有工作区文件。默认安装包和程序资源可以共用，安装脚本与下载必须带有效探针密钥，旧版仅凭源 IP 下载的兼容方式不再使用。

## 备份与回滚

升级前备份配置、Compose 文件、所有 SQLite 数据库与工作区归属标记、主题目录，以及 VictoriaMetrics 快照。SQLite 应用在线备份或停机后完整复制数据库/WAL，不能在写入时单独拷贝 `.db` 文件。先在副本验证，再替换应用镜像。

首次升级可保留原镜像用于回退。如果已经启用多个账号，不得直接回退到旧的共享数据版本并保留所有用户配置；旧版不会执行指标过滤，可能暴露其他工作区的指标。回退时应停机恢复升级前的完整配置及指标快照，只保留原拥有者账号。

## 验证

```sh
go test ./internal/... ./pkg/agent/...
go test -race ./internal ./internal/vmclient ./internal/handler
# 以下测试会写入并删除测试指标，只能连接一次性测试数据库：
YUANLOOK_TEST_VM_URL=http://127.0.0.1:18428 go test ./internal/vmclient -run TestWorkspaceIsolationWithVictoriaMetrics -v
```

测试还覆盖注册、管理员授权、密码修改与重置、启停与会话撤销、并发创建、配置重复导入，以及双账号真实接口、猜测资源 ID、缓存与配置隔离、密钥读取/修改/撤销、移除账号后旧凭据失效、同 ID 探针分别注册，以及真实 VictoriaMetrics 中对同 ID 指标的隔离删除。

账号管理上线后不要直接回退到只读取配置文件账号的旧版：旧版会忽略网页上更改的密码和停用状态。需要回退时先关闭入口并恢复匹配版本的完整备份。
