package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/artemstarovojtovigorevich-eng/go-udstream/pkg/udstream"
	pb "github.com/artemstarovojtovigorevich-eng/go-udstream/proto/pb/proto"
	"opcua-proxy21/internal/logger"
	"opcua-proxy21/internal/opcua"
	"opcua-proxy21/internal/store"
)

type Handler struct {
	log   *logger.Logger
	store *store.Store
	srv   *opcua.UDPServer
}

func (h *Handler) OnDelta(delta *pb.DeltaBatch) {
	h.log.Info("DELTA received",
		"seq", delta.Seq,
		"source", getSourceIDs(delta.Messages),
		"count", len(delta.Messages),
	)

	messages := make([]*opcua.UDMessage, len(delta.Messages))
	for i, msg := range delta.Messages {
		messages[i] = convertMessage(msg)
		if i < 3 {
			h.logMessage(msg)
		}
	}
	if len(delta.Messages) > 3 {
		h.log.Info("... and more messages", "remaining", len(delta.Messages)-3)
	}

	h.srv.HandleDelta(messages)
}

func (h *Handler) OnFull(full *pb.FullSnapshot) {
	h.log.Info("FULL received",
		"timestamp", full.Timestamp,
		"source", full.SourceId,
		"count", len(full.Nodes),
	)

	messages := make([]*opcua.UDMessage, len(full.Nodes))
	for i, msg := range full.Nodes {
		messages[i] = convertMessage(msg)
		if i < 5 {
			h.logMessage(msg)
		}
	}
	if len(full.Nodes) > 5 {
		h.log.Info("... and more nodes", "remaining", len(full.Nodes)-5)
	}

	h.srv.HandleFull(messages)
}

func getSourceIDs(msgs []*pb.Message) uint32 {
	if len(msgs) == 0 {
		return 0
	}
	return msgs[0].SourceId
}

func (h *Handler) logMessage(msg *pb.Message) {
	val := formatValue(msg.Value)
	h.log.Info("node",
		"node_id", msg.NodeId,
		"value", val,
		"timestamp_ns", msg.TimestampNs,
	)
}

func convertMessage(msg *pb.Message) *opcua.UDMessage {
	return &opcua.UDMessage{
		NodeId:      msg.NodeId,
		Value:       convertValue(msg.Value),
		TimestampNs: msg.TimestampNs,
		Quality:     0,
	}
}

func convertValue(v *pb.Value) interface{} {
	if v == nil {
		return nil
	}
	switch x := v.Value.(type) {
	case *pb.Value_DoubleValue:
		return x.DoubleValue
	case *pb.Value_Int64Value:
		return x.Int64Value
	case *pb.Value_Uint64Value:
		return x.Uint64Value
	case *pb.Value_BoolValue:
		return x.BoolValue
	case *pb.Value_StringValue:
		return x.StringValue
	case *pb.Value_BytesValue:
		return x.BytesValue
	case *pb.Value_FloatValue:
		return x.FloatValue
	default:
		return nil
	}
}

func formatValue(v *pb.Value) string {
	if v == nil {
		return "nil"
	}
	switch x := v.Value.(type) {
	case *pb.Value_DoubleValue:
		return fmt.Sprintf("%.6f", x.DoubleValue)
	case *pb.Value_Int64Value:
		return fmt.Sprintf("%d", x.Int64Value)
	case *pb.Value_Uint64Value:
		return fmt.Sprintf("%d", x.Uint64Value)
	case *pb.Value_BoolValue:
		return fmt.Sprintf("%v", x.BoolValue)
	case *pb.Value_StringValue:
		return fmt.Sprintf("%q", x.StringValue)
	case *pb.Value_BytesValue:
		return fmt.Sprintf("[%d bytes]", len(x.BytesValue))
	case *pb.Value_FloatValue:
		return fmt.Sprintf("%.6f", x.FloatValue)
	default:
		return "unknown"
	}
}

func main() {
	addr := flag.String("addr", ":50001", "UDStream listen address")
	opcAddr := flag.String("opc-addr", "0.0.0.0", "OPC UA server address")
	opcPort := flag.Int("opc-port", 4841, "OPC UA server port")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logEncoding := flag.String("log-encoding", "console", "Log encoding: console, json")

	flag.Parse()

	log, err := logger.New(*logLevel, *logEncoding)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st := store.New()
	srv, err := opcua.NewUDPServer(*opcAddr, *opcPort, log)
	if err != nil {
		log.Error("Failed to create OPC UA server", "error", err)
		os.Exit(1)
	}

	if err := srv.Start(ctx); err != nil {
		log.Error("Failed to start OPC UA server", "error", err)
		os.Exit(1)
	}
	defer srv.Close()

	handler := &Handler{log: log, store: st, srv: srv}
	udserver, err := udstream.NewServer(&udstream.Config{Addr: *addr}, handler)
	if err != nil {
		log.Error("Failed to create UDStream server", "error", err)
		os.Exit(1)
	}

	log.Info("Starting servers",
		"udstream", *addr,
		"opcua", fmt.Sprintf("opc.tcp://%s:%d", *opcAddr, *opcPort),
	)
	log.Info("Press Ctrl+C to stop")

	go func() {
		if err := udserver.Start(ctx); err != nil {
			if err == context.Canceled {
				log.Info("UDStream server stopped")
			} else {
				log.Error("UDStream server error", "error", err)
			}
		}
	}()

	<-ctx.Done()
	log.Info("Shutdown complete")
}
