# YuanLook

**Private server monitoring, under your control.** Prefer not to self-host? Visit [look.zjpb.net](https://look.zjpb.net) and [register directly](https://look.zjpb.net/admin/register) to get your own isolated monitoring workspace.

[简体中文](README.zh-CN.md) · [Private mode](docs/private-mode.md) · [License](LICENSE)

YuanLook is a self-hosted server monitoring and management project based on [Pika](https://github.com/pika-monitor/pika). This repository is public; deployed dashboards and monitoring data require authentication.

## What changes

- Server-side login checks protect the dashboard, server details and management pages.
- Host lists, tags, metrics and service-monitoring APIs reject anonymous, forged and expired credentials, including for legacy records marked public.
- Password sign-in establish an HttpOnly browser session; administrative mutations retain Bearer-token/API-key authentication.
- New resources default to private, and the management UI no longer offers anonymous visibility.
- Probe registration retains its API-key authentication and does not require a browser session.

See [private-mode behavior and boundaries](docs/private-mode.md), including the minimal public login/bootstrap and agent-download endpoints.

## Isolated accounts

Each configured password account owns an independent workspace: SQLite database, service caches, probe connections, themes, configuration and scheduled jobs. Metric reads/deletes are scoped to that workspace. The original database and historical unlabelled metrics remain with `admin`; new accounts start empty. The legacy owner has no cross-account access.

Workspace mode currently requires SQLite and configured password accounts. OAuth-enabled or non-SQLite configurations fail closed. Multiple accounts require authenticated VictoriaMetrics storage. See [workspace operations and upgrade notes](docs/workspaces.md).

## Features

Go agents and server, a React/TypeScript management UI, SQLite for business data, and VictoriaMetrics for time-series metrics. Inherited features include CPU/memory/disk/network monitoring, HTTP/TCP/ICMP checks, notifications, DDNS, SSH login monitoring, tamper protection and Linux asset auditing.

## Build from source

YuanLook does not yet publish its own container image. The inherited Compose files reference **upstream Pika images**, which do not contain YuanLook's private-mode changes. Build this source before deployment.

Requirements: Go 1.26+, Node.js 22+, npm and make. A running VictoriaMetrics instance is required for metrics.

```sh
git clone https://github.com/Selei1983/YuanLook.git
cd YuanLook
git clone https://github.com/pika-monitor/pika-default-theme.git ../pika-default-theme
git -C ../pika-default-theme checkout "$(cat .github/default-theme.ref)"
make build-web
go build -o bin/yuanlook ./cmd/serv
cp config.sqlite.yaml config.yaml
```

Edit `config.yaml`: set your admin password hash, JWT secret and VictoriaMetrics URL. Then run:

```sh
./bin/yuanlook serve --config config.yaml
```

Build the required agents from `./cmd/agent`, or use `make build-agents` with UPX installed for the full upstream platform matrix. Built-in agent downloads require those binaries in `bin/agents`.

The Go module path, agent protocol, service names and parts of the UI retain upstream naming for compatibility. The default theme is a separately maintained Pika project, pinned by `.github/default-theme.ref`.

## Validation

```sh
go test ./internal/... ./pkg/agent/...
npm ci --prefix web
npm run build --prefix web
npm run lint --prefix web
```

Private mode has been deployed and verified. Workspace tests cover account isolation, key revocation, probe registration and metric boundaries. Third-party OAuth is not enabled in workspace mode. Upstream image/release publishing workflows are restricted to the upstream repository; YuanLook runs a separate validation workflow.

## Attribution

Derived from Pika commit `f391cb15a60efdc979672baba915cb85e67e8711`. Original copyright and Apache-2.0 license are preserved. YuanLook is an independent derivative and is not an official Pika release. See [UPSTREAM.md](UPSTREAM.md).

## Account registration and management

Users can register at `/admin/register` and immediately receive an isolated workspace. The legacy owner can create accounts, enable/disable login access and reset passwords at `/admin/accounts`; all users can change their own password from the profile menu. Existing configuration accounts are imported once into the primary SQLite database. Later password and status changes persist across restarts. Password changes and status updates revoke existing user sessions.

Registration is enabled by default; set `App.Workspaces.RegistrationEnabled: false` to close it. Registration or multiple accounts require authenticated VictoriaMetrics storage. Disabling an account preserves its data and background monitoring; see [workspace operations and backup notes](docs/workspaces.md).
