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
		"discoverNodes", cfg.GetDiscoverNodes(),
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

	var nodes []opcua.NodeInfo
	if app.cfg.GetDiscoverNodes() {
		app.log.Info("Starting full node discovery with metadata")
		app.reader = opcua.NewReader(app.opcClient, nil, app.log)

		if err := app.reader.DiscoverNodes(ctx); err != nil {
			app.log.Error("Node discovery failed", "error", err)
			return err
		}
		nodes = app.reader.GetNodeInfos()

		app.log.Info("Discovered nodes", "count", len(nodes))
		for _, n := range nodes {
			app.log.Debug("Found node",
				"id", n.ID,
				"name", n.Name,
				"data_type", n.DataType,
			)
		}
	} else {
		nodes = []opcua.NodeInfo{
			{ID: "ns=3;s=FastUInt1", Name: "FastUInt1", DataType: "UInt32"},
			{ID: "ns=3;s=SlowUInt1", Name: "SlowUInt1", DataType: "UInt32"},
			{ID: "ns=3;s=StepUp", Name: "StepUp", DataType: "UInt32"},
			{ID: "ns=3;s=AlternatingBoolean", Name: "AlternatingBoolean", DataType: "Boolean"},
			{ID: "ns=3;s=RandomSignedInt32", Name: "RandomSignedInt32", DataType: "Int32"},
		}
		app.log.Info("Using hardcoded nodes", "count", len(nodes))
	}

	if app.reader == nil {
		app.reader = opcua.NewReader(app.opcClient, nil, app.log)
	}

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
			if app.cfg.GetDiscoverNodes() {
				data, err := app.reader.ReadDiscoveredNodes(ctx)
				if err != nil {
					app.log.Error("Failed to read data", "error", err)
					continue
				}

				app.log.Debug("Read completed", "count", len(data))

				if len(data) > 0 {
					sendData := data
					if len(sendData) > 15 {
						sendData = sendData[:15]
					}

					if app.cfg.GetReadOnly() {
						app.log.Info("Read values", "count", len(sendData))
					} else {
						if err := app.sender.SendNodesWithMetadata(app.cfg.GetOPCEndpoint(), sendData); err != nil {
							app.log.Error("Failed to send", "error", err)
						} else {
							app.log.Info("Sent", "count", len(sendData))
						}
					}
				}
			} else {
				// hardcoded mode: read known nodes
				nodes := []opcua.Node{
					{ID: "ns=3;s=FastUInt1", Name: "FastUInt1"},
					{ID: "ns=3;s=SlowUInt1", Name: "SlowUInt1"},
					{ID: "ns=3;s=StepUp", Name: "StepUp"},
					{ID: "ns=3;s=AlternatingBoolean", Name: "AlternatingBoolean"},
					{ID: "ns=3;s=RandomSignedInt32", Name: "RandomSignedInt32"},
				}
				data, err := app.reader.ReadMultiple(ctx, nodes)
				if err != nil {
					app.log.Error("Failed to read data", "error", err)
					continue
				}

				if len(data) > 0 {
					// convert DataValue to NodeInfo for sender
					nodeInfos := make([]opcua.NodeInfo, len(data))
					for i, d := range data {
						nodeInfos[i] = opcua.NodeInfo{
							ID:        d.NodeID,
							Value:     d.Value,
							Timestamp: d.Timestamp,
						}
					}

					if app.cfg.GetReadOnly() {
						app.log.Info("Read values", "count", len(data))
					} else {
						if err := app.sender.SendNodesWithMetadata(app.cfg.GetOPCEndpoint(), nodeInfos); err != nil {
							app.log.Error("Failed to send", "error", err)
						} else {
							app.log.Info("Sent", "count", len(data))
						}
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