package opcua

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua/ua"
)

type Node struct {
	ID   string
	Name string
}

type DataValue struct {
	NodeID    string
	Value     interface{}
	Timestamp time.Time
}

type Reader struct {
	client *Client
	nodes  []Node
	logger Logger
}

func NewReader(client *Client, nodes []Node, logger Logger) *Reader {
	return &Reader{
		client: client,
		nodes:  nodes,
		logger: logger,
	}
}

func (r *Reader) Read(ctx context.Context) ([]DataValue, error) {
	if r.client.client == nil {
		return nil, fmt.Errorf("client not connected")
	}

	var results []DataValue
	for _, node := range r.nodes {
		nodeID, err := ua.ParseNodeID(node.ID)
		if err != nil {
			r.logger.Warn("Failed to parse node ID", "node", node.ID, "error", err)
			continue
		}

		val, err := r.client.client.Node(nodeID).Value(ctx)
		if err != nil {
			r.logger.Debug("Skipping node - not a variable or error", "node", node.ID, "error", err)
			continue
		}

		if val != nil {
			results = append(results, DataValue{
				NodeID:    node.ID,
				Value:     val.Value(),
				Timestamp: time.Now(),
			})
		}
	}

	return results, nil
}

func (r *Reader) ReadMultiple(ctx context.Context, nodes []Node) ([]DataValue, error) {
	if r.client.client == nil {
		return nil, fmt.Errorf("client not connected")
	}

	if len(nodes) == 0 {
		return nil, nil
	}

	req := &ua.ReadRequest{
		MaxAge: 0,
		NodesToRead: make([]*ua.ReadValueID, 0, len(nodes)),
	}

	for _, node := range nodes {
		req.NodesToRead = append(req.NodesToRead, &ua.ReadValueID{
			NodeID:       ua.MustParseNodeID(node.ID),
			AttributeID: ua.AttributeIDValue,
		})
	}

	resp, err := r.client.client.Read(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	var results []DataValue
	for i, dr := range resp.Results {
		if dr.Status != ua.StatusOK {
			continue
		}
		if dr.Value == nil {
			continue
		}
		if i < len(nodes) {
			results = append(results, DataValue{
				NodeID:    nodes[i].ID,
				Value:     dr.Value.Value(),
				Timestamp: time.Now(),
			})
		}
	}

	return results, nil
}

func (r *Reader) BrowseAllNodes(ctx context.Context, browsePath string, namespaceFilter int) ([]Node, error) {
	if r.client.client == nil {
		return nil, fmt.Errorf("client not connected")
	}

	rootNode, err := ua.ParseNodeID(browsePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse browse path: %w", err)
	}

	var nodes []Node
	rootRef := r.client.client.Node(rootNode)
	refs, err := rootRef.ReferencedNodes(ctx, 0, ua.BrowseDirectionForward, ua.NodeClassAll, true)
	if err != nil {
		return nil, fmt.Errorf("failed to browse: %w", err)
	}

	for _, ref := range refs {
		refNode := r.client.client.Node(ref.ID)
		children, err := refNode.ReferencedNodes(ctx, 0, ua.BrowseDirectionForward, ua.NodeClassVariable, true)
		if err != nil {
			continue
		}

		for _, child := range children {
			idStr := child.ID.String()
			ns := child.ID.Namespace()

			if namespaceFilter > 0 && int(ns) != namespaceFilter {
				continue
			}

			attrs, err := child.Attributes(ctx, ua.AttributeIDBrowseName)
			if err != nil {
				nodes = append(nodes, Node{ID: idStr, Name: idStr})
				continue
			}

			name := attrs[0].Value.String()
			if name == "" {
				name = idStr
			}

			nodes = append(nodes, Node{ID: idStr, Name: name})
		}
	}

	return nodes, nil
}
