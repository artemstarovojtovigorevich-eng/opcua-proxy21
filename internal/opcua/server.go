package opcua

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/server/attrs"
	"github.com/gopcua/opcua/ua"
	"opcua-proxy21/internal/store"
)

type UDPServer struct {
	srv    *server.Server
	ns     server.NameSpace
	store  *store.Store
	logger Logger
	ready  chan struct{}
}

type ServerLogger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

func NewUDPServer(opcAddr string, opcPort int, logger ServerLogger) (*UDPServer, error) {
	host := opcAddr
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	opts := []server.Option{
		server.EnableSecurity("None", ua.MessageSecurityModeNone),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
		server.EndPoint(host, opcPort),
	}

	s := server.New(opts...)

	srv := &UDPServer{
		srv:    s,
		store:  store.New(),
		logger: logger,
		ready:  make(chan struct{}),
	}

	return srv, nil
}

func (s *UDPServer) Start(ctx context.Context) error {
	if err := s.srv.Start(ctx); err != nil {
		return fmt.Errorf("failed to start OPC UA server: %w", err)
	}

	ns := server.NewNodeNameSpace(s.srv, "UDStream")
	s.ns = ns

	nsID := ns.ID()
	s.logger.Info("OPC UA server started", "namespace", nsID)

	srvNS, _ := s.srv.Namespace(0)
	rootObj := srvNS.Objects()
	nnsObj := ns.Objects()
	rootObj.AddRef(nnsObj, id.HasComponent, true)

	// Log all registered endpoints
	endpoints := s.srv.Endpoints()
	s.logger.Info("Server endpoints registered", "count", len(endpoints))
	for i, ep := range endpoints {
		s.logger.Info("Endpoint",
			"index", i,
			"url", ep.EndpointURL,
			"security", ep.SecurityPolicyURI,
			"mode", ep.SecurityMode,
		)
	}

	close(s.ready)
	return nil
}

func (s *UDPServer) Close() error {
	return s.srv.Close()
}

func (s *UDPServer) HandleFull(messages []*UDMessage) {
	<-s.ready

	for _, msg := range messages {
		s.updateNode(msg)
	}
	s.logger.Info("Full snapshot processed", "count", len(messages))
}

func (s *UDPServer) HandleDelta(messages []*UDMessage) {
	<-s.ready

	for _, msg := range messages {
		s.updateNode(msg)
	}
	s.logger.Info("Delta processed", "count", len(messages))
}

func (s *UDPServer) updateNode(msg *UDMessage) {
	nodeID, err := ua.ParseNodeID(msg.NodeId)
	if err != nil {
		s.logger.Error("Failed to parse node ID", "node", msg.NodeId, "error", err)
		return
	}

	existingNode := s.ns.Node(nodeID)
	if existingNode != nil {
		s.updateNodeValue(nodeID, msg)
		return
	}

	s.createNode(nodeID, msg)
}

func (s *UDPServer) createNode(nodeID *ua.NodeID, msg *UDMessage) {
	varName := nodeID.String()
	if sid := nodeID.StringID(); sid != "" {
		varName = sid
	}

	if msg.BrowseName != "" {
		varName = msg.BrowseName
	}

	dispName := attrs.BrowseName(varName)
	dataType := getDataTypeFromString(msg.DataType, msg.Value)

	node := server.NewNode(
		nodeID,
		map[ua.AttributeID]*ua.DataValue{
			ua.AttributeIDBrowseName:  server.DataValueFromValue(dispName),
			ua.AttributeIDNodeClass:   server.DataValueFromValue(uint32(ua.NodeClassVariable)),
			ua.AttributeIDDataType:    server.DataValueFromValue(dataType),
			ua.AttributeIDAccessLevel: server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead | ua.AccessLevelTypeHistoryRead)),
		},
		nil,
		func() *ua.DataValue {
			val, ok := s.store.Get(nodeID.String())
			if !ok {
				return server.DataValueFromValue(nil)
			}
			return toUADataValue(val)
		},
	)

	s.ns.AddNode(node)

	objects := s.ns.Objects()
	objects.AddRef(node, id.HasComponent, true)

	timestamp := time.Unix(0, int64(msg.TimestampNs))
	quality := uint16(msg.Quality)
	if quality == 0 {
		quality = uint16(ua.StatusOK)
	}

	s.store.Update(nodeID.String(), msg.Value, timestamp, quality)

	s.logger.Debug("Node created", "node_id", nodeID.String(), "name", varName)
}

func (s *UDPServer) updateNodeValue(nodeID *ua.NodeID, msg *UDMessage) {
	timestamp := time.Unix(0, int64(msg.TimestampNs))
	quality := uint16(msg.Quality)
	if quality == 0 {
		quality = uint16(ua.StatusOK)
	}

	s.store.Update(nodeID.String(), msg.Value, timestamp, quality)
}

func getDataType(v interface{}) *ua.ExpandedNodeID {
	var typeID uint32
	switch v.(type) {
	case bool:
		typeID = id.Boolean
	case int8, int16, int32, int64, int:
		typeID = id.Int64
	case uint8, uint16, uint32, uint64, uint:
		typeID = id.UInt64
	case float32:
		typeID = id.Float
	case float64:
		typeID = id.Double
	case string:
		typeID = id.String
	case []byte:
		typeID = id.ByteString
	default:
		typeID = id.String
	}
	return ua.NewExpandedNodeID(ua.NewNumericNodeID(0, typeID), "", typeID)
}

func getDataTypeFromString(dataType string, v interface{}) *ua.ExpandedNodeID {
	typeMap := map[string]uint32{
		"Boolean":   id.Boolean,
		"Int16":    id.Int16,
		"UInt16":   id.UInt16,
		"Int32":    id.Int32,
		"UInt32":   id.UInt32,
		"Int64":    id.Int64,
		"UInt64":   id.UInt64,
		"Float":    id.Float,
		"Double":   id.Double,
		"String":   id.String,
		"DateTime": id.DateTime,
		"ByteString": id.ByteString,
	}

	if typeID, ok := typeMap[dataType]; ok {
		return ua.NewExpandedNodeID(ua.NewNumericNodeID(0, typeID), "", typeID)
	}

	return getDataType(v)
}

func toUADataValue(val *store.DataValue) *ua.DataValue {
	dv := &ua.DataValue{
		SourceTimestamp: val.SourceTimestamp,
		EncodingMask:    ua.DataValueValue | ua.DataValueSourceTimestamp,
	}
	dv.Value = ua.MustVariant(val.Value)
	return dv
}

type UDMessage struct {
	NodeId      string
	Value       interface{}
	TimestampNs uint64
	Quality     uint16
	BrowseName  string
	DataType    string
}
