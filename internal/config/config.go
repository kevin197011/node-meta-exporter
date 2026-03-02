package config

import (
	"fmt"
	"time"
)

// Config holds all configuration for the exporter.
type Config struct {
	ListenAddress string
	MetricsPath   string

	JumpServerURL string
	AccessKeyID   string
	AccessKeySecret string
	OrgID         string

	ScrapeInterval time.Duration
	RequestTimeout time.Duration
	PageSize       int

	TLSInsecureSkipVerify bool
	ActiveOnly            bool
}

// Validate checks that all required configuration fields are set.
func (c *Config) Validate() error {
	if c.JumpServerURL == "" {
		return fmt.Errorf("jumpserver URL is required")
	}
	if c.AccessKeyID == "" {
		return fmt.Errorf("access key ID is required")
	}
	if c.AccessKeySecret == "" {
		return fmt.Errorf("access key secret is required")
	}
	return nil
}
