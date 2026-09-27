# YuanLook

**Private server monitoring, under your control.**

[简体中文](README.zh-CN.md) · [Private mode](docs/private-mode.md) · [License](LICENSE)

YuanLook is a self-hosted server monitoring and management project based on [Pika](https://github.com/pika-monitor/pika). This repository is public; deployed dashboards and monitoring data require authentication.

## What changes

- Server-side login checks protect the dashboard, server details and management pages.
- Host lists, tags, metrics and service-monitoring APIs reject anonymous, forged and expired credentials, including for legacy records marked public.
- Password, OIDC and GitHub sign-in establish an HttpOnly browser session; administrative mutations retain Bearer-token/API-key authentication.
- New resources default to private, and the management UI no longer offers anonymous visibility.
- Probe registration retains its API-key authentication and does not require a browser session.

See [private-mode behavior and boundaries](docs/private-mode.md), including the minimal public login/bootstrap and agent-download endpoints.

## Features

Go agents and server, a React/TypeScript management UI, SQLite or PostgreSQL for business data, and VictoriaMetrics for time-series metrics. Inherited features include CPU/memory/disk/network monitoring, HTTP/TCP/ICMP checks, notifications, DDNS, SSH login monitoring, tamper protection and Linux asset auditing.

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

These checks passed locally for the private-mode implementation. Real-server deployment and third-party OAuth provider flows have not yet been exercised. Upstream image/release publishing workflows are restricted to the upstream repository; YuanLook runs a separate validation workflow.

## Attribution

Derived from Pika commit `f391cb15a60efdc979672baba915cb85e67e8711`. Original copyright and Apache-2.0 license are preserved. YuanLook is an independent derivative and is not an official Pika release. See [UPSTREAM.md](UPSTREAM.md).
