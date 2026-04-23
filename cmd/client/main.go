package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"opcua-proxy21/internal/config"
	"opcua-proxy21/internal/logger"
	"opcua-proxy21/internal/opcua"
	"opcua-proxy21/internal/sender"
	"opcua-proxy21/pkg/cert"
)

type App struct {
	cfg       *config.Config
	log       *logger.Logger
	opcClient *opcua.Client
	reader    *opcua.Reader
	sender    *sender.UDStreamSender
	certMgr   *cert.CertManager
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Config validation failed: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(cfg.GetLogLevel(), cfg.GetLogEncoding())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("Starting OPC UA Proxy",
		"endpoint", cfg.GetOPCEndpoint(),
		"udp", cfg.GetUDPDest(),
		"sourceID", cfg.GetSourceID(),
		"pollInterval", cfg.GetPollInterval(),
		"readonly", cfg.GetReadOnly(),
	)

	app := NewApp(cfg, log)
	if err := app.Start(ctx); err != nil {
		log.Fatal("Failed to start application", "error", err)
	}

	<-ctx.Done()
	log.Info("Shutting down...")

	if err := app.Shutdown(context.Background()); err != nil {
		log.Error("Error during shutdown", "error", err)
	}
}

func NewApp(cfg *config.Config, log *logger.Logger) *App {
	certMgr := cert.NewCertManager(cfg.GetCertFile(), cfg.GetKeyFile(), "urn:client")
	return &App{
		cfg:     cfg,
		log:     log,
		certMgr: certMgr,
	}
}

func (app *App) Start(ctx context.Context) error {
	certData, privKey, err := app.certMgr.LoadOrGenerate(app.cfg.GetGenCert())
	if err != nil {
		return err
	}

	// Try discovery first
	discovery := opcua.NewDiscovery(app.cfg.GetOPCEndpoint(), app.log)
	endpoints, err := discovery.GetEndpoints(ctx)
	if err != nil {
		app.log.Warn("Discovery failed, trying direct connect", "error", err)
		app.opcClient = opcua.NewClientDirect(app.cfg.GetOPCEndpoint(), app.log)
	} else {
		endpoint := discovery.FindEndpoint(endpoints, app.cfg.GetSecurityPolicy(), app.cfg.GetSecurityMode())
		if endpoint != nil {
			app.log.Info("Using endpoint",
				"url", endpoint.EndpointURL,
				"security", endpoint.SecurityPolicyURI,
				"mode", endpoint.SecurityMode,
			)
			app.opcClient = opcua.NewClient(app.cfg.GetOPCEndpoint(), endpoint, certData, privKey, app.log)
		} else if len(endpoints) > 0 {
			app.log.Warn("No matching endpoint, using first available")
			app.opcClient = opcua.NewClientDirect(app.cfg.GetOPCEndpoint(), app.log)
		} else {
			app.log.Warn("No endpoints found, trying direct connect")
			app.opcClient = opcua.NewClientDirect(app.cfg.GetOPCEndpoint(), app.log)
		}
	}

	if err := app.opcClient.Connect(ctx); err != nil {
		return err
	}

	var nodes []opcua.Node
	if app.cfg.GetDiscoverNodes() {
		app.log.Info("Starting node discovery", "browsePath", app.cfg.GetBrowsePath(), "namespace", app.cfg.GetNodeNamespace())
		browsePath := app.cfg.GetBrowsePath()
		namespaceFilter := app.cfg.GetNodeNamespace()

		discoveredNodes, err := opcua.NewReader(app.opcClient, nil, app.log).BrowseAllNodes(ctx, browsePath, namespaceFilter)
		if err != nil {
			app.log.Error("Node discovery failed", "error", err)
			return err
		}

		app.log.Info("Discovered nodes", "count", len(discoveredNodes))
		for _, n := range discoveredNodes {
			app.log.Debug("Found node", "id", n.ID, "name", n.Name)
		}
		nodes = discoveredNodes
	} else {
		nodes = []opcua.Node{
			{ID: "ns=3;s=FastUInt1", Name: "FastUInt1"},
			{ID: "ns=3;s=SlowUInt1", Name: "SlowUInt1"},
			{ID: "ns=3;s=StepUp", Name: "StepUp"},
			{ID: "ns=3;s=AlternatingBoolean", Name: "AlternatingBoolean"},
			{ID: "ns=3;s=RandomSignedInt32", Name: "RandomSignedInt32"},
		}
		app.log.Info("Using hardcoded nodes", "count", len(nodes))
	}

	app.reader = opcua.NewReader(app.opcClient, nodes, app.log)

	if !app.cfg.GetReadOnly() {
		app.sender = sender.NewUDStreamSender(app.cfg.GetUDPDest(), app.cfg.GetSourceID())
		if err := app.sender.Connect(); err != nil {
			return err
		}
	}

	go app.pollLoop(ctx)

	return nil
}

func (app *App) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(app.cfg.GetPollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := app.reader.Read(ctx)
			if err != nil {
				app.log.Error("Failed to read data", "error", err)
				continue
			}

			if len(data) > 0 {
				if app.cfg.GetReadOnly() {
					app.log.Info("Read values", "count", len(data))
					for _, d := range data {
						app.log.Debug("Node value",
							"node_id", d.NodeID,
							"value", fmt.Sprintf("%v", d.Value),
						)
					}
				} else {
					if err := app.sender.Send(app.cfg.GetOPCEndpoint(), data); err != nil {
						app.log.Error("Failed to send", "error", err)
					} else {
						app.log.Info("Data sent", "count", len(data))
					}
				}
			}
		}
	}
}

func (app *App) Shutdown(ctx context.Context) error {
	app.log.Info("Closing OPC client...")
	if app.opcClient != nil {
		_ = app.opcClient.Disconnect(ctx)
	}

	if !app.cfg.GetReadOnly() && app.sender != nil {
		app.log.Info("Closing UDP sender...")
		_ = app.sender.Close()
	}

	app.log.Info("Shutdown complete")
	return nil
}
