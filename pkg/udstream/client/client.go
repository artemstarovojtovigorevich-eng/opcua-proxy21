package client

import (
	"context"
	"net"
	"sync"

	"opcua-proxy21/pkg/udstream/pb"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	conn   *net.UDPConn
	remote *net.UDPAddr
	srcID  uint32
	mutex  sync.Mutex
}

func NewClient(dstAddr string, srcID uint32) (*Client, error) {
	remote, err := net.ResolveUDPAddr("udp", dstAddr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		remote: remote,
		srcID:  srcID,
		mutex:  sync.Mutex{},
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) send(packet *pb.Packet) error {
	data, err := proto.Marshal(packet)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, err = c.conn.Write(data)
	return err
}

func (c *Client) SendDelta(ctx context.Context, seq uint32, msgs []*pb.Message) error {
	batch := &pb.DeltaBatch{
		Seq:      seq,
		Messages: msgs,
	}

	packet := &pb.Packet{
		Payload: &pb.Packet_Delta{Delta: batch},
	}

	return c.send(packet)
}

func (c *Client) SendFull(ctx context.Context, timestamp uint64, nodes []*pb.Message) error {
	full := &pb.FullSnapshot{
		Timestamp: timestamp,
		SourceId:  c.srcID,
		Nodes:     nodes,
	}

	packet := &pb.Packet{
		Payload: &pb.Packet_Full{Full: full},
	}

	return c.send(packet)
}

func (c *Client) Addr() *net.UDPAddr {
	return c.remote
}

func (c *Client) SrcID() uint32 {
	return c.srcID
}