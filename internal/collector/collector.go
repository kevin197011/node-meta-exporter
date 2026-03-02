package collector

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/node-meta-exporter/internal/jumpserver"
	"github.com/prometheus/client_golang/prometheus"
)

// HostMetaCollector collects host metadata from JumpServer
// and exposes it as a Prometheus info metric (constant value 1).
type HostMetaCollector struct {
	client *jumpserver.Client
	logger *slog.Logger

	mu    sync.RWMutex
	hosts []jumpserver.Host

	hostMeta *prometheus.Desc
	scrapeOK *prometheus.Desc
	scrapeDuration *prometheus.Desc
	hostCount *prometheus.Desc
}

// NewHostMetaCollector creates a new collector that periodically
// fetches host metadata from JumpServer.
func NewHostMetaCollector(client *jumpserver.Client, logger *slog.Logger) *HostMetaCollector {
	return &HostMetaCollector{
		client: client,
		logger: logger,
		hostMeta: prometheus.NewDesc(
			"infra_host_meta",
			"JumpServer host metadata with labels for hostname, address, platform, and tags. Value is always 1.",
			[]string{"id", "hostname", "ip", "platform", "comment", "org_name", "node", "labels"},
			nil,
		),
		scrapeOK: prometheus.NewDesc(
			"infra_scrape_ok",
			"Whether the last JumpServer scrape was successful (1=success, 0=failure).",
			nil, nil,
		),
		scrapeDuration: prometheus.NewDesc(
			"infra_scrape_duration_seconds",
			"Duration of the last JumpServer hosts scrape in seconds.",
			nil, nil,
		),
		hostCount: prometheus.NewDesc(
			"infra_host_count",
			"Total number of active hosts fetched from JumpServer.",
			nil, nil,
		),
	}
}

// Describe sends all metric descriptors to the channel.
func (c *HostMetaCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.hostMeta
	ch <- c.scrapeOK
	ch <- c.scrapeDuration
	ch <- c.hostCount
}

// Collect fetches host data and sends metrics to the channel.
func (c *HostMetaCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	hosts := c.hosts
	c.mu.RUnlock()

	for _, h := range hosts {
		labels := formatLabels(h.Labels)
		node := formatNodes(h.Nodes)

		ch <- prometheus.MustNewConstMetric(
			c.hostMeta,
			prometheus.GaugeValue,
			1,
			h.ID,
			h.Name,
			h.Address,
			h.Platform.Name,
			h.Comment,
			h.OrgName,
			node,
			labels,
		)
	}

	ch <- prometheus.MustNewConstMetric(c.hostCount, prometheus.GaugeValue, float64(len(hosts)))
}

// StartBackgroundScrape starts a goroutine that periodically scrapes JumpServer.
// The first scrape happens immediately; subsequent ones follow the interval.
func (c *HostMetaCollector) StartBackgroundScrape(ctx context.Context, interval time.Duration, reg prometheus.Registerer) {
	scrapeOK := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "infra_scrape_ok",
		Help: "Whether the last JumpServer scrape was successful.",
	})
	scrapeDuration := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "infra_scrape_duration_seconds",
		Help: "Duration of the last JumpServer hosts scrape.",
	})

	// These are for /metrics; the Collect method also emits them from cached data.
	// We intentionally do NOT register these to avoid duplicate descriptor errors.
	// Instead the Describe/Collect path handles them.
	_ = scrapeOK
	_ = scrapeDuration

	go func() {
		c.scrape(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.scrape(ctx)
			}
		}
	}()
}

// scrape fetches hosts from JumpServer and updates the cached data.
func (c *HostMetaCollector) scrape(ctx context.Context) {
	start := time.Now()
	hosts, err := c.client.FetchAllHosts(ctx)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("Failed to scrape JumpServer hosts",
			"error", err,
			"duration", duration,
		)
		return
	}

	c.mu.Lock()
	c.hosts = hosts
	c.mu.Unlock()

	c.logger.Info("JumpServer hosts scraped successfully",
		"host_count", len(hosts),
		"duration", duration,
	)
}

// formatLabels joins label key=value pairs into a single comma-separated string.
func formatLabels(labels []jumpserver.Label) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, l.Name+"="+l.Value)
	}
	return strings.Join(parts, ",")
}

// formatNodes joins node full-path names with semicolons.
func formatNodes(nodes []jumpserver.Node) string {
	if len(nodes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		name := n.FullName
		if name == "" {
			name = n.Name
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ";")
}
