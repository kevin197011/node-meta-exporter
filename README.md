# Node Meta Exporter

A Prometheus exporter that collects host metadata from [JumpServer](https://www.jumpserver.org/) and exposes it as `infra_host_meta` info metrics. This enables hostname-to-IP mapping, tag-based filtering, and platform identification in Prometheus for relabeling and service discovery.

The metric name uses a generic `infra_` prefix so that the data source can be swapped (e.g. from JumpServer to another CMDB) without changing downstream PromQL or alert rules.

## Features

- Fetches hosts from JumpServer `/api/v1/assets/hosts/` with automatic pagination
- Uses **Access Key** (HTTP Signature / HMAC-SHA256) authentication — no password required
- Exposes `infra_host_meta` gauge metric (value=1) with rich label dimensions
- Configurable active-only or all-hosts mode
- Background scraping with configurable interval (avoids slow `/metrics` responses)
- Health check endpoint at `/healthz`
- Multi-arch Docker image (amd64/arm64) with GitHub Actions CI/CD

## Metrics

### `infra_host_meta`

| Label      | Description                                   |
|------------|-----------------------------------------------|
| `id`       | JumpServer asset UUID                         |
| `hostname` | Host display name                             |
| `ip`       | IP address or FQDN                            |
| `platform` | OS platform (e.g. `Linux`, `Windows`)         |
| `comment`  | Asset comment/description                     |
| `org_name` | Organization name                             |
| `node`     | Asset tree node path (`;` separated)          |
| `labels`   | Key=Value label pairs (`,` separated)         |

### Other Metrics

| Metric                              | Description                         |
|-------------------------------------|-------------------------------------|
| `infra_host_count`                  | Total hosts fetched                 |
| `infra_scrape_ok`                   | Last scrape success (1/0)           |
| `infra_scrape_duration_seconds`     | Last scrape duration                |

## Quick Start

### 1. Create Access Key in JumpServer

1. Login to JumpServer as admin
2. Go to **Personal Settings** > **Access Keys**
3. Create a new access key and note the **Key ID** and **Key Secret**

### 2. Configure

```bash
cp .env.example .env
# Edit .env with your JumpServer URL and access key credentials
```

### 3. Run with Docker Compose

```bash
docker-compose up -d
```

The exporter will be available at `http://localhost:9101/metrics`.

### 4. Run with Docker (from GitHub Packages)

```bash
docker run -d --name node-meta-exporter \
  -p 9101:9101 \
  -e JUMPSERVER_URL=https://jms.example.com \
  -e JUMPSERVER_ACCESS_KEY_ID=your-key-id \
  -e JUMPSERVER_ACCESS_KEY_SECRET=your-key-secret \
  ghcr.io/<owner>/node-meta-exporter:latest
```

### 5. Run Locally (Development)

```bash
export JUMPSERVER_URL=https://jms.example.com
export JUMPSERVER_ACCESS_KEY_ID=your-key-id
export JUMPSERVER_ACCESS_KEY_SECRET=your-key-secret

go run ./cmd/node-meta-exporter/
```

## Configuration

All settings can be provided via CLI flags or environment variables:

| Flag                        | Env Var                        | Default       | Description                                       |
|-----------------------------|--------------------------------|---------------|---------------------------------------------------|
| `--listen-address`          | `LISTEN_ADDRESS`               | `:9101`       | Listen address                                    |
| `--metrics-path`            | `METRICS_PATH`                 | `/metrics`    | Metrics endpoint path                             |
| `--jumpserver-url`          | `JUMPSERVER_URL`               | (required)    | JumpServer base URL                               |
| `--access-key-id`           | `JUMPSERVER_ACCESS_KEY_ID`     | (required)    | Access key ID                                     |
| `--access-key-secret`       | `JUMPSERVER_ACCESS_KEY_SECRET` | (required)    | Access key secret                                 |
| `--org-id`                  | `JUMPSERVER_ORG_ID`            | (empty=all)   | Organization ID, empty fetches all orgs           |
| `--scrape-interval`         | `SCRAPE_INTERVAL`              | `5m`          | Background scrape interval                        |
| `--request-timeout`         | `REQUEST_TIMEOUT`              | `30s`         | HTTP request timeout                              |
| `--page-size`               | `PAGE_SIZE`                    | `100`         | Hosts per API page                                |
| `--active-only`             | `ACTIVE_ONLY`                  | `true`        | Only fetch active hosts (set `false` for all)     |
| `--tls-insecure-skip-verify`| `TLS_INSECURE_SKIP_VERIFY`     | `false`       | Skip TLS cert verification                        |

## Prometheus 配置

在 Prometheus 的 `prometheus.yml` 中增加抓取 node-meta-exporter 的 job，例如：

### 单实例（静态配置）

```yaml
scrape_configs:
  - job_name: "infra-host-meta"
    scrape_interval: 1h
    scrape_timeout: 30s
    static_configs:
      - targets: ["node-meta-exporter:9101"]   # 同机/同 compose 用服务名
        labels:
          env: production
```

### 多实例（多 target）

```yaml
scrape_configs:
  - job_name: "infra-host-meta"
    scrape_interval: 1h
    scrape_timeout: 30s
    static_configs:
      - targets:
          - "10.0.1.10:9101"
          - "10.0.1.11:9101"
        labels:
          project: infra
```

### 使用服务发现（如 Kubernetes）

```yaml
scrape_configs:
  - job_name: "infra-host-meta"
    scrape_interval: 1h
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names: ["monitoring"]
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: keep
        regex: node-meta-exporter
      - source_labels: [__meta_kubernetes_pod_ip]
        target_label: __address__
        replacement: "${1}:9101"
      - source_labels: [__meta_kubernetes_namespace]
        target_label: kubernetes_namespace
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: kubernetes_pod_name
```

抓取成功后即可在 Prometheus 中查询 `infra_host_meta`、`infra_host_count` 等指标。

## PromQL Join Examples

Use `infra_host_meta` to enrich other metrics with hostname. Since Prometheus's `instance` label typically includes a port (e.g. `10.0.1.100:9100`), use `label_replace` to extract the IP before joining:

### Join node_exporter metrics with hostname

```promql
label_replace(node_load1, "ip", "$1", "instance", "(.+):.*")
  * on(ip) group_left(hostname, platform)
infra_host_meta
```

### Recording rule for persistent hostname mapping

```yaml
groups:
  - name: infra_meta_join
    rules:
      - record: node_meta_info
        expr: |
          label_replace(node_uname_info, "ip", "$1", "instance", "(.+):.*")
            * on(ip) group_left(hostname, platform, labels)
          infra_host_meta
```

### Alert with hostname

```yaml
groups:
  - name: host_alerts
    rules:
      - alert: HighCPULoad
        expr: |
          (
            label_replace(node_load1, "ip", "$1", "instance", "(.+):.*")
              * on(ip) group_left(hostname)
            infra_host_meta
          ) > 10
        labels:
          severity: warning
        annotations:
          summary: "High CPU load on {{ $labels.hostname }} ({{ $labels.ip }})"
```

## Project Structure

```
.
├── cmd/node-meta-exporter/       # Application entry point
│   └── main.go
├── internal/
│   ├── collector/                # Prometheus collector
│   │   └── collector.go
│   ├── config/                   # Configuration types
│   │   └── config.go
│   └── jumpserver/               # JumpServer API client
│       ├── client.go
│       └── signer.go             # HTTP Signature (HMAC-SHA256) auth
├── configs/
│   └── prometheus.yml            # Sample Prometheus config
├── .github/workflows/
│   └── build-and-push.yml        # CI/CD: build & push to ghcr.io
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── go.mod
└── README.md
```

## Build

```bash
# Local build
go build -o node-meta-exporter ./cmd/node-meta-exporter/

# Docker build
docker build -t node-meta-exporter:latest --build-arg VERSION=$(git describe --tags --always) .
```

## CI/CD

Pushing to `main` or tagging `v*` triggers GitHub Actions to build a multi-arch Docker image and push it to GitHub Packages (ghcr.io). See `.github/workflows/build-and-push.yml`.

## License

MIT
