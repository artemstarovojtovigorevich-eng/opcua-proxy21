package sender

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/artemstarovojtovigorevich-eng/go-udstream/pkg/udstream"
	pb "github.com/artemstarovojtovigorevich-eng/go-udstream/proto/pb/proto"
	"opcua-proxy21/internal/opcua"
)

type UDStreamSender struct {
	addr     string
	sourceID uint32
	client   *udstream.Client
}

func NewUDStreamSender(addr string, sourceID uint32) *UDStreamSender {
	return &UDStreamSender{
		addr:     addr,
		sourceID: sourceID,
	}
}

func (s *UDStreamSender) Connect() error {
	client, err := udstream.NewClient(s.addr, s.sourceID)
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

func (s *UDStreamSender) Send(endpoint string, data []opcua.DataValue) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	messages := make([]*pb.Message, len(data))
	for i, dv := range data {
		messages[i] = convertDataValue(dv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	timestamp := uint64(time.Now().UnixNano())
	return s.client.SendFull(ctx, timestamp, messages)
}

func convertDataValue(dv opcua.DataValue) *pb.Message {
	msg := &pb.Message{
		TimestampNs: uint64(dv.Timestamp.UnixNano()),
		NodeId:      dv.NodeID,
		SourceId:    0,
		Value:       convertValue(dv.Value),
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
