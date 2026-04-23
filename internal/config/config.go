package config

import (
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"time"
)

func generateSourceID() uint32 {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "default"
	}
	h := fnv.New32a()
	h.Write([]byte(hostname))
	return h.Sum32()
}

type Config struct {
	OPCEndpoint     *string
	UDPDest         *string
	PollInterval    *time.Duration
	CertFile        *string
	KeyFile         *string
	GenCert         *bool
	SecurityMode   *string
	SecurityPolicy  *string
	LogLevel       *string
	LogEncoding    *string
	DiscoverNodes  *bool
	BrowsePath     *string
	NodeNamespace  *int
	MaxDepth       *int
	ReadOnly       *bool
	SourceID       uint32
}

func Load() *Config {
	cfg := &Config{
		OPCEndpoint:     flag.String("endpoint", "opc.tcp://localhost:50000", "OPC UA Endpoint URL"),
		UDPDest:         flag.String("udp", "localhost:50001", "UDP destination address"),
PollInterval: flag.Duration("poll-interval", 20*time.Millisecond, "Poll interval"),
		CertFile:        flag.String("cert", "cert.pem", "Path to certificate file"),
		KeyFile:         flag.String("key", "key.pem", "Path to PEM Private Key file"),
		GenCert:         flag.Bool("gen-cert", false, "Generate a new certificate"),
		SecurityMode:   flag.String("sec-mode", "Sign", "Security Mode: None, Sign, SignAndEncrypt"),
		SecurityPolicy:  flag.String("sec-policy", "Basic256Sha256", "Security Policy"),
		LogLevel:       flag.String("log-level", "info", "Log level: debug, info, warn, error"),
		LogEncoding:    flag.String("log-encoding", "console", "Log encoding: console, json"),
		DiscoverNodes:  flag.Bool("discover-nodes", false, "Enable full auto-discovery of all Variable nodes from all namespaces"),
		BrowsePath:     flag.String("browse-path", "ns=0;i=85", "Browse path for node discovery (e.g., ns=0;i=85 for Objects)"),
		NodeNamespace:  flag.Int("node-namespace", 0, "Filter by namespace (0 = all namespaces)"),
		MaxDepth:       flag.Int("max-depth", 0, "Max recursion depth for discovery (0 = unlimited)"),
		ReadOnly:       flag.Bool("readonly", false, "Read only mode - don't send to UDP, just log values"),
	}
	cfg.SourceID = generateSourceID()

	flag.Parse()

	if envEndpoint := os.Getenv("OPC_ENDPOINT"); envEndpoint != "" {
		*cfg.OPCEndpoint = envEndpoint
	}
	if envUDP := os.Getenv("UDP_DEST"); envUDP != "" {
		*cfg.UDPDest = envUDP
	}
	if envPoll := os.Getenv("POLL_INTERVAL"); envPoll != "" {
		if d, err := time.ParseDuration(envPoll); err == nil {
			*cfg.PollInterval = d
		}
	}

	return cfg
}

func (c *Config) Validate() error {
	if *c.OPCEndpoint == "" {
		return fmt.Errorf("OPC endpoint is required")
	}
	if !*c.ReadOnly && *c.UDPDest == "" {
		return fmt.Errorf("UDP destination is required")
	}
	if *c.PollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}
	return nil
}

func (c *Config) GetOPCEndpoint() string         { return *c.OPCEndpoint }
func (c *Config) GetUDPDest() string             { return *c.UDPDest }
func (c *Config) GetPollInterval() time.Duration { return *c.PollInterval }
func (c *Config) GetCertFile() string            { return *c.CertFile }
func (c *Config) GetKeyFile() string            { return *c.KeyFile }
func (c *Config) GetGenCert() bool               { return *c.GenCert }
func (c *Config) GetSecurityMode() string        { return *c.SecurityMode }
func (c *Config) GetSecurityPolicy() string      { return *c.SecurityPolicy }
func (c *Config) GetLogLevel() string           { return *c.LogLevel }
func (c *Config) GetLogEncoding() string        { return *c.LogEncoding }
func (c *Config) GetDiscoverNodes() bool        { return *c.DiscoverNodes }
func (c *Config) GetBrowsePath() string         { return *c.BrowsePath }
func (c *Config) GetNodeNamespace() int          { return *c.NodeNamespace }
func (c *Config) GetMaxDepth() int               { return *c.MaxDepth }
func (c *Config) GetReadOnly() bool              { return *c.ReadOnly }
func (c *Config) GetSourceID() uint32            { return c.SourceID }
