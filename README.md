# Node Meta Exporter

A Prometheus exporter that collects host metadata from [JumpServer](https://www.jumpserver.org/) and exposes it as `jumpserver_host_meta` info metrics. This enables hostname-to-IP mapping, tag-based filtering, and platform identification in Prometheus for relabeling and service discovery.

## Features

- Fetches all active hosts from JumpServer `/api/v1/assets/hosts/` with automatic pagination
- Uses **Access Key** (HTTP Signature / HMAC-SHA256) authentication — no password required
- Exposes `jumpserver_host_meta` gauge metric (value=1) with rich label dimensions
- Background scraping with configurable interval (avoids slow `/metrics` responses)
- Health check endpoint at `/healthz`
- Docker & Docker Compose ready

## Metrics

### `jumpserver_host_meta`

| Label      | Description                                   |
|------------|-----------------------------------------------|
| `id`       | JumpServer asset UUID                         |
| `hostname` | Host display name                             |
| `address`  | IP address or FQDN                            |
| `platform` | OS platform (e.g. `Linux`, `Windows`)         |
| `comment`  | Asset comment/description                     |
| `org_name` | Organization name                             |
| `node`     | Asset tree node path (`;` separated)          |
| `labels`   | Key=Value label pairs (`,` separated)         |

### Other Metrics

| Metric                              | Description                         |
|-------------------------------------|-------------------------------------|
| `jumpserver_host_count`             | Total active hosts fetched          |

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

The exporter will be available at `http://localhost:9101/metrics` and a Prometheus instance at `http://localhost:9090`.

### 4. Run Locally (Development)

```bash
export JUMPSERVER_URL=https://jms.example.com
export JUMPSERVER_ACCESS_KEY_ID=your-key-id
export JUMPSERVER_ACCESS_KEY_SECRET=your-key-secret

go run ./cmd/node-meta-exporter/
```

## Configuration

All settings can be provided via CLI flags or environment variables:

| Flag                        | Env Var                        | Default                                      | Description                          |
|-----------------------------|--------------------------------|----------------------------------------------|--------------------------------------|
| `--listen-address`          | `LISTEN_ADDRESS`               | `:9101`                                      | Listen address                       |
| `--metrics-path`            | `METRICS_PATH`                 | `/metrics`                                   | Metrics endpoint path                |
| `--jumpserver-url`          | `JUMPSERVER_URL`               | (required)                                   | JumpServer base URL                  |
| `--access-key-id`           | `JUMPSERVER_ACCESS_KEY_ID`     | (required)                                   | Access key ID                        |
| `--access-key-secret`       | `JUMPSERVER_ACCESS_KEY_SECRET` | (required)                                   | Access key secret                    |
| `--org-id`                  | `JUMPSERVER_ORG_ID`            | `00000000-0000-0000-0000-000000000002`       | Organization ID                      |
| `--scrape-interval`         | `SCRAPE_INTERVAL`              | `5m`                                         | Background scrape interval           |
| `--request-timeout`         | `REQUEST_TIMEOUT`              | `30s`                                        | HTTP request timeout                 |
| `--page-size`               | `PAGE_SIZE`                    | `100`                                        | Hosts per API page                   |
| `--tls-insecure-skip-verify`| `TLS_INSECURE_SKIP_VERIFY`     | `false`                                      | Skip TLS cert verification           |

## Prometheus Relabeling Example

Use `jumpserver_host_meta` to enrich other metrics (e.g. `node_exporter`) with hostname:

```yaml
# In Prometheus scrape config for node_exporter
metric_relabel_configs:
  # Join jumpserver_host_meta with node metrics by IP address
  - source_labels: [instance]
    regex: '(.+):.*'
    target_label: __tmp_instance
    replacement: '$1'
```

Or use Prometheus `group_left` in recording rules:

```yaml
groups:
  - name: host_meta_join
    rules:
      - record: node_meta_info
        expr: |
          node_uname_info
          * on(instance) group_left(hostname, platform, labels)
          jumpserver_host_meta
```

## Project Structure

```
.
├── cmd/node-meta-exporter/    # Application entry point
│   └── main.go
├── internal/
│   ├── collector/             # Prometheus collector implementation
│   │   └── collector.go
│   ├── config/                # Configuration types
│   │   └── config.go
│   └── jumpserver/            # JumpServer API client
│       ├── client.go
│       └── signer.go          # HTTP Signature (HMAC-SHA256) auth
├── configs/
│   └── prometheus.yml         # Sample Prometheus config
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

## License

MIT
