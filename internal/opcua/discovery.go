package opcua

import (
	"context"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

type Discovery struct {
	endpoint string
	logger   Logger
}

type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

func NewDiscovery(endpoint string, logger Logger) *Discovery {
	logger.Info("Creating discovery for endpoint", "endpoint", endpoint)
	return &Discovery{
		endpoint: endpoint,
		logger:   logger,
	}
}

func (d *Discovery) GetEndpoints(ctx context.Context) ([]*ua.EndpointDescription, error) {
	d.logger.Info("Discovering OPC UA endpoints", "endpoint", d.endpoint)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	d.logger.Debug("Attempting to connect and get endpoints...")

	endpoints, err := opcua.GetEndpoints(ctx, d.endpoint)
	if err != nil {
		d.logger.Error("Failed to get endpoints", "error", err)
		return nil, err
	}

	d.logger.Info("Found endpoints", "count", len(endpoints))
	for i, ep := range endpoints {
		d.logger.Debug("Endpoint",
			"index", i,
			"url", ep.EndpointURL,
			"security_mode", ep.SecurityMode,
			"security_policy", ep.SecurityPolicyURI,
		)
	}

	return endpoints, nil
}

func (d *Discovery) FindEndpoint(endpoints []*ua.EndpointDescription, securityPolicy, securityMode string) *ua.EndpointDescription {
	desiredMode := ua.MessageSecurityModeNone
	switch securityMode {
	case "None":
		desiredMode = ua.MessageSecurityModeNone
	case "Sign":
		desiredMode = ua.MessageSecurityModeSign
	case "SignAndEncrypt":
		desiredMode = ua.MessageSecurityModeSignAndEncrypt
	}

	desiredPolicy := "http://opcfoundation.org/UA/SecurityPolicy#" + securityPolicy

	d.logger.Debug("Finding endpoint",
		"desired_policy", desiredPolicy,
		"desired_mode", desiredMode,
		"available_count", len(endpoints),
	)

	for _, ep := range endpoints {
		d.logger.Debug("Checking endpoint",
			"policy", ep.SecurityPolicyURI,
			"mode", ep.SecurityMode,
			"url", ep.EndpointURL,
		)

		if ep.SecurityPolicyURI != desiredPolicy {
			continue
		}
		if ep.SecurityMode != desiredMode {
			continue
		}

		return ep
	}

	// No exact match - try None mode as fallback
	if securityMode != "None" {
		for _, ep := range endpoints {
			if ep.SecurityPolicyURI == "http://opcfoundation.org/UA/SecurityPolicy#None" &&
				ep.SecurityMode == ua.MessageSecurityModeNone {
				d.logger.Info("Using fallback endpoint with None security")
				return ep
			}
		}
	}

	// Last resort - return first available
	if len(endpoints) > 0 {
		d.logger.Warn("No exact match, using first available endpoint")
		return endpoints[0]
	}

	return nil
}
