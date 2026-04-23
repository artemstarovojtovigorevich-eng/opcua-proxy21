package sender

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"opcua-proxy21/pkg/udstream/client"
	pb "opcua-proxy21/pkg/udstream/pb"
	"opcua-proxy21/internal/opcua"
)

type UDStreamSender struct {
	addr     string
	sourceID uint32
	client   *client.Client
}

func NewUDStreamSender(addr string, sourceID uint32) *UDStreamSender {
	return &UDStreamSender{
		addr:     addr,
		sourceID: sourceID,
	}
}

func (s *UDStreamSender) Connect() error {
	client, err := client.NewClient(s.addr, s.sourceID)
	if err != nil {
		return fmt.Errorf("failed to create udstream client: %w", err)
	}
	s.client = client
	return nil
}

func (s *UDStreamSender) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func (s *UDStreamSender) SendNodesWithMetadata(endpoint string, nodes []opcua.NodeInfo) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	if len(nodes) == 0 {
		return nil
	}

	const maxBatch = 5
	batches := (len(nodes) + maxBatch - 1) / maxBatch
	for i := 0; i < len(nodes); i += maxBatch {
		end := i + maxBatch
		if end > len(nodes) {
			end = len(nodes)
		}

		batch := nodes[i:end]
		messages := make([]*pb.Message, len(batch))
		for j, node := range batch {
			messages[j] = convertNodeInfo(node)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		timestamp := uint64(time.Now().UnixNano())
		batchNum := i / maxBatch
		if err := s.client.SendFull(ctx, timestamp, messages); err != nil {
			return fmt.Errorf("batch %d failed: %w", batchNum, err)
		}
	}

	fmt.Printf("Total sent: %d nodes in %d batches\n", len(nodes), batches)
	return nil
}

func (s *UDStreamSender) SendDelta(endpoint string, data []opcua.DataValue) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	messages := make([]*pb.Message, len(data))
	for i, dv := range data {
		messages[i] = convertNodeInfo(opcua.NodeInfo{
			ID:        dv.NodeID,
			Value:     dv.Value,
			Timestamp: dv.Timestamp,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	timestamp := uint64(time.Now().UnixNano())
	return s.client.SendFull(ctx, timestamp, messages)
}

func convertNodeInfo(node opcua.NodeInfo) *pb.Message {
	msg := &pb.Message{
		TimestampNs: uint64(node.Timestamp.UnixNano()),
		NodeId:      node.ID,
		SourceId:    0,
		Value:       convertValue(node.Value),
		Metadata: &pb.NodeMetadata{
			BrowseName: node.BrowseName,
			DataType: node.DataType,
		},
	}
	return msg
}

func convertValue(v interface{}) *pb.Value {
	if v == nil {
		return nil
	}

	val := &pb.Value{}
	rv := reflect.ValueOf(v)

	switch v.(type) {
	case bool:
		val.Value = &pb.Value_BoolValue{BoolValue: v.(bool)}
	case int:
		val.Value = &pb.Value_Int64Value{Int64Value: int64(v.(int))}
	case int8:
		val.Value = &pb.Value_Int64Value{Int64Value: int64(v.(int8))}
	case int16:
		val.Value = &pb.Value_Int64Value{Int64Value: int64(v.(int16))}
	case int32:
		val.Value = &pb.Value_Int64Value{Int64Value: int64(v.(int32))}
	case int64:
		val.Value = &pb.Value_Int64Value{Int64Value: v.(int64)}
	case uint:
		val.Value = &pb.Value_Uint64Value{Uint64Value: uint64(v.(uint))}
	case uint8:
		val.Value = &pb.Value_Uint64Value{Uint64Value: uint64(v.(uint8))}
	case uint16:
		val.Value = &pb.Value_Uint64Value{Uint64Value: uint64(v.(uint16))}
	case uint32:
		val.Value = &pb.Value_Uint64Value{Uint64Value: uint64(v.(uint32))}
	case uint64:
		val.Value = &pb.Value_Uint64Value{Uint64Value: v.(uint64)}
	case float32:
		val.Value = &pb.Value_DoubleValue{DoubleValue: float64(v.(float32))}
	case float64:
		val.Value = &pb.Value_DoubleValue{DoubleValue: v.(float64)}
	case string:
		val.Value = &pb.Value_StringValue{StringValue: v.(string)}
	case []byte:
		val.Value = &pb.Value_BytesValue{BytesValue: v.([]byte)}
	default:
		if rv.Kind() == reflect.Ptr {
			elem := rv.Elem()
			if elem.IsValid() {
				return convertValue(elem.Interface())
			}
		}
		val.Value = &pb.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}
	}

	return val
}