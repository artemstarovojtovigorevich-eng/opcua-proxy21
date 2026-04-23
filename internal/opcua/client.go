package opcua

import (
	"context"
	"crypto/rsa"
	"fmt"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

type Client struct {
	client     *opcua.Client
	endpoint   string
	endpointDP *ua.EndpointDescription
	cert       []byte
	privKey    *rsa.PrivateKey
	logger     Logger
}

func NewClient(endpoint string, ep *ua.EndpointDescription, cert []byte, privKey *rsa.PrivateKey, logger Logger) *Client {
	return &Client{
		endpoint:   endpoint,
		endpointDP: ep,
		cert:       cert,
		privKey:    privKey,
		logger:     logger,
	}
}

func NewClientDirect(endpoint string, logger Logger) *Client {
	return &Client{
		endpoint: endpoint,
		logger:  logger,
	}
}

func (c *Client) Connect(ctx context.Context) error {
	if c.endpointDP != nil {
		opts := []opcua.Option{
			opcua.PrivateKey(c.privKey),
			opcua.Certificate(c.cert),
			opcua.SecurityFromEndpoint(c.endpointDP, ua.UserTokenTypeAnonymous),
		}

		client, err := opcua.NewClient(c.endpoint, opts...)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		c.client = client

		if err := client.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		c.logger.Info("Connected to OPC UA server", "endpoint", c.endpoint)
		return nil
	}

	opts := []opcua.Option{
		opcua.SecurityPolicy("None"),
	}

	client, err := opcua.NewClient(c.endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	c.client = client

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.logger.Info("Connected to OPC UA server", "endpoint", c.endpoint)
	return nil
}

func (c *Client) Disconnect(ctx context.Context) error {
	if c.client != nil {
		c.client.Close(ctx)
		c.logger.Info("Disconnected from OPC UA server")
	}
	return nil
}

func (c *Client) NamespaceArray(ctx context.Context) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client not connected")
	}
	return c.client.NamespaceArray(ctx)
}
