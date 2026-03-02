package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/node-meta-exporter/internal/collector"
	"github.com/node-meta-exporter/internal/config"
	"github.com/node-meta-exporter/internal/jumpserver"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version = "dev"

func main() {
	cfg := parseFlags()
	logger := initLogger()

	if err := cfg.Validate(); err != nil {
		logger.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("Starting node-meta-exporter",
		"version", version,
		"listen", cfg.ListenAddress,
		"jumpserver_url", cfg.JumpServerURL,
		"scrape_interval", cfg.ScrapeInterval,
	)

	client := jumpserver.NewClient(
		cfg.JumpServerURL,
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
		jumpserver.WithTimeout(cfg.RequestTimeout),
		jumpserver.WithInsecureSkipVerify(cfg.TLSInsecureSkipVerify),
		jumpserver.WithOrgID(cfg.OrgID),
		jumpserver.WithPageSize(cfg.PageSize),
		jumpserver.WithActiveOnly(cfg.ActiveOnly),
		jumpserver.WithLogger(logger),
	)

	hostCollector := collector.NewHostMetaCollector(client, logger)

	reg := prometheus.NewRegistry()
	reg.MustRegister(hostCollector)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostCollector.StartBackgroundScrape(ctx, cfg.ScrapeInterval, reg)

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<html><head><title>Node Meta Exporter</title></head>
<body><h1>Node Meta Exporter</h1>
<p>Version: %s</p>
<p><a href="%s">Metrics</a></p>
</body></html>`, version, cfg.MetricsPath)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	server := &http.Server{
		Addr:         cfg.ListenAddress,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("Listening", "address", cfg.ListenAddress, "metrics_path", cfg.MetricsPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-sigCh
	logger.Info("Received shutdown signal", "signal", sig)

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}
	logger.Info("Exporter stopped")
}

func parseFlags() *config.Config {
	cfg := &config.Config{}

	flag.StringVar(&cfg.ListenAddress, "listen-address", envOrDefault("LISTEN_ADDRESS", ":9101"), "Address to listen on for metrics")
	flag.StringVar(&cfg.MetricsPath, "metrics-path", envOrDefault("METRICS_PATH", "/metrics"), "Path under which to expose metrics")
	flag.StringVar(&cfg.JumpServerURL, "jumpserver-url", os.Getenv("JUMPSERVER_URL"), "JumpServer base URL (e.g. https://jms.example.com)")
	flag.StringVar(&cfg.AccessKeyID, "access-key-id", os.Getenv("JUMPSERVER_ACCESS_KEY_ID"), "JumpServer access key ID")
	flag.StringVar(&cfg.AccessKeySecret, "access-key-secret", os.Getenv("JUMPSERVER_ACCESS_KEY_SECRET"), "JumpServer access key secret")
	flag.StringVar(&cfg.OrgID, "org-id", os.Getenv("JUMPSERVER_ORG_ID"), "JumpServer organization ID (empty=all orgs)")

	scrapeInterval := flag.Duration("scrape-interval", parseDurationOrDefault(os.Getenv("SCRAPE_INTERVAL"), 5*time.Minute), "Interval between JumpServer API scrapes")
	requestTimeout := flag.Duration("request-timeout", parseDurationOrDefault(os.Getenv("REQUEST_TIMEOUT"), 30*time.Second), "HTTP request timeout for JumpServer API calls")
	flag.IntVar(&cfg.PageSize, "page-size", envIntOrDefault("PAGE_SIZE", 100), "Number of hosts per API page request")
	flag.BoolVar(&cfg.TLSInsecureSkipVerify, "tls-insecure-skip-verify", os.Getenv("TLS_INSECURE_SKIP_VERIFY") == "true", "Skip TLS certificate verification")
	flag.BoolVar(&cfg.ActiveOnly, "active-only", os.Getenv("ACTIVE_ONLY") != "false", "Only fetch active hosts (set ACTIVE_ONLY=false to include inactive)")

	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("node-meta-exporter %s\n", version)
		os.Exit(0)
	}

	cfg.ScrapeInterval = *scrapeInterval
	cfg.RequestTimeout = *requestTimeout

	return cfg
}

func initLogger() *slog.Logger {
	env := os.Getenv("ENV")
	var handler slog.Handler
	opts := &slog.HandlerOptions{AddSource: false}

	if env == "production" {
		opts.Level = slog.LevelInfo
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		opts.Level = slog.LevelDebug
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}

func parseDurationOrDefault(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
