# 多账号私有工作区

## 数据归属与升级

`App.Users` 中每个密码账号对应一个独立工作区。浏览器使用已验证的 JWT/Cookie 选择工作区；管理 API Key、探针注册密钥和下载密钥按实际所在数据库选择工作区，客户端不能用用户 ID、Header 或 URL 参数切换归属。

原 `database.sqlite.path` 数据库、原主题目录和原有无 `yuanlook_workspace` 标签的指标固定归属于 `App.Workspaces.LegacyOwner`（默认 `admin`）。原来的服务器 ID、监控 ID、密钥和数据保留，无需重装探针。首次启动生成 `<原数据库路径>.workspace-owner`，防止后续误改归属；备份时必须一起保留。

其他账号的数据存储在 `App.Workspaces.Dir/<用户名 SHA256>/pika.db`，主题目录位于同级 `themes`。每个账号使用独立服务实例、缓存、WebSocket 管理器和定时任务。配置文件、JWT 签名密钥、默认主题程序文件及指标数据库服务本身由部署管理员统一维护。

旧指标不改写；新账号指标强制写入其 `yuanlook_workspace` 标签。包括聚合查询、标签枚举、按主机删除和清理孤立数据在内的读写操作均限定工作区。应用只支持 VictoriaMetrics，不能用忽略 `extra_filters[]` 的兼容 API 替代；不得开启 VM 的 `-search.ignoreExtraFiltersAtLabelsAPI`。

## 添加账号

当前通过服务器配置文件维护账号，没有开放注册或网页用户管理。账号名请使用小写字母、数字、`_`、`-`，账号名是持久身份，不要通过重命名来修改显示名称。

1. 使用 `htpasswd -nBC 12 alice` 交互生成密码哈希。不要把明文密码写到命令行、GitHub 或日志里。
2. 在 `App.Users` 保留原账号并增加 `alice: '<生成的 bcrypt 哈希>'`。
3. 给 VictoriaMetrics 配置服务端认证，并在 `App.VictoriaMetrics.Username`、`Password` 中填写同一组凭据。这是存储服务专用凭据，不是登录密码，不能提供给工作区用户。
4. 重启 YuanLook。账号可以在同一登录页登录，新账号得到空白工作区。在该账号自己的设置中填写服务端安装地址，然后创建自己的探针密钥。

部署配置结构：

```yaml
App:
  Users:
    admin: '<已有管理员 bcrypt 哈希>'
    alice: '<新用户 bcrypt 哈希>'
  Workspaces:
    LegacyOwner: admin
    Dir: ./data/workspaces
  VictoriaMetrics:
    Enabled: true
    URL: http://victoriametrics:8428
    Username: yuanlook-storage
    Password: '<随机生成的存储服务密码>'
```

VM 容器需添加 `-httpAuth.username=yuanlook-storage`、`-httpAuth.password=file:///run/secrets/vm-password`，把密码文件只读挂载到该路径，文件内容与应用配置一致。保持 VM 仅在内部网络可达。多账号启动时会检查未认证查询返回 401、应用凭据查询成功；检查不通过则拒绝启动。此保护还防止用户可配置的 Webhook 绕过应用直接操作内部指标库。

账号从配置中删除并重启后，旧 JWT、Cookie 和该账号密钥不再能路由到工作区。数据库文件不会删除，重新加入同名账号可以恢复，因此不要把已使用的账号名分配给不同的人。退出登录沿用原 JWT 有效期语义，复制出的 Token 不会因退出立即失效。

## 兼容范围

当前支持 SQLite + 密码账号。OIDC、GitHub OAuth、非 SQLite 数据库会明确拒绝启动，后续需要独立设计身份与空间绑定，不能直接打开原来的 OAuth 开关。`admin` 只是旧数据所有者，不具备查看其他工作区的网页超级管理员权限。

本项目仍是由可信部署管理员管理账号的自托管工具，不提供公共注册、计费或每用户 CPU/内存配额。主机管理员能读取所有工作区文件。默认安装包和程序资源可以共用，安装脚本与下载必须带有效探针密钥，旧版仅凭源 IP 下载的兼容方式不再使用。

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

测试覆盖双账号真实接口、猜测资源 ID、缓存与配置隔离、密钥读取/修改/撤销、移除账号后旧凭据失效、同 ID 探针分别注册，以及真实 VictoriaMetrics 中对同 ID 指标的隔离删除。
