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

type NodeInfo struct {
	ID        string
	Name      string
	BrowseName string
	DataType  string
	Value    interface{}
	Timestamp time.Time
}

type DataValue struct {
	NodeID    string
	Value     interface{}
	Timestamp time.Time
}

type Reader struct {
	client     *Client
	nodes      []Node
	nodeInfos  []NodeInfo
	logger    Logger
}

func NewReader(client *Client, nodes []Node, logger Logger) *Reader {
	return &Reader{
		client:   client,
		nodes:    nodes,
		nodeInfos: []NodeInfo{},
		logger:   logger,
	}
}

func (r *Reader) GetNodeInfos() []NodeInfo {
	return r.nodeInfos
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
	nodes, err := r.BrowseAllNamespaces(ctx, browsePath, 0)
	if err != nil {
		return nil, err
	}

	var result []Node
	for _, n := range nodes {
		id, err := ua.ParseNodeID(n.ID)
		if err != nil {
			continue
		}
		ns := id.Namespace()

		if namespaceFilter > 0 && int(ns) != namespaceFilter {
			continue
		}

		result = append(result, Node{ID: n.ID, Name: n.Name})
	}

	return result, nil
}

func (r *Reader) BrowseAllNamespaces(ctx context.Context, browsePath string, maxDepth int) ([]NodeInfo, error) {
	if r.client.client == nil {
		return nil, fmt.Errorf("client not connected")
	}

	namespaces, err := r.client.NamespaceArray(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace array: %w", err)
	}

	r.logger.Info("Discovered namespaces", "count", len(namespaces))

	var allNodes []NodeInfo

	rootPaths := []string{
		"ns=0;i=85",
		"ns=0;i=86",
		"ns=0;i=87",
	}

	for _, path := range rootPaths {
		nodes, err := r.browseRecursive(ctx, path, maxDepth, 0)
		if err != nil {
			r.logger.Warn("Failed to browse path", "path", path, "error", err)
			continue
		}
		allNodes = append(allNodes, nodes...)
	}

r.logger.Info("Total discovered nodes", "count", len(allNodes))
	return allNodes, nil
}

func (r *Reader) browseRecursive(ctx context.Context, browsePath string, maxDepth int, currentDepth int) ([]NodeInfo, error) {
	if maxDepth > 0 && currentDepth >= maxDepth {
		return nil, nil
	}

	rootNode, err := ua.ParseNodeID(browsePath)
	if err != nil {
		return nil, err
	}

	var nodes []NodeInfo
	queue := []*ua.NodeID{rootNode}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		visitedKey := currentID.String()
		if visited[visitedKey] {
			continue
		}
		visited[visitedKey] = true

		refs, err := r.client.client.Node(currentID).ReferencedNodes(ctx, 0, ua.BrowseDirectionForward, ua.NodeClassAll, true)
		if err != nil {
			continue
		}

		for _, ref := range refs {
			ns := ref.ID.Namespace()
			if ns != 2 && ns != 3 {
				continue
			}

			attrs, err := r.client.client.Node(ref.ID).Attributes(ctx, ua.AttributeIDNodeClass)
			if err != nil {
				continue
			}
			if len(attrs) == 0 || attrs[0].Status != ua.StatusOK {
				continue
			}

			nodeClass := ua.NodeClass(attrs[0].Value.Int())

			if nodeClass == ua.NodeClassVariable {
				info := r.getVariableInfo(ctx, ref.ID)
				if info.ID != "" {
					nodes = append(nodes, info)
				}
			} else if nodeClass == ua.NodeClassObject {
				queue = append(queue, ref.ID)
			}
		}
	}

	return nodes, nil
}

func (r *Reader) getVariableInfo(ctx context.Context, nodeID *ua.NodeID) NodeInfo {
	info := NodeInfo{ID: nodeID.String()}

	attrs, err := r.client.client.Node(nodeID).Attributes(ctx,
		ua.AttributeIDBrowseName,
		ua.AttributeIDDataType,
	)
	if err != nil {
		return info
	}

	if len(attrs) >= 1 && attrs[0].Status == ua.StatusOK && attrs[0].Value != nil {
		info.BrowseName = attrs[0].Value.String()
		if info.BrowseName != "" {
			info.Name = info.BrowseName
		} else {
			info.Name = info.ID
		}
	}

	if len(attrs) >= 2 && attrs[1].Status == ua.StatusOK && attrs[1].Value != nil {
		nid := attrs[1].Value.NodeID()
		info.DataType = string(nid.String())
	}

	val, err := r.client.client.Node(nodeID).Value(ctx)
	if err == nil && val != nil {
		info.Value = val.Value()
		info.Timestamp = time.Now()
	}

	return info
}

func dataTypeToString(typeID uint32) string {
	switch typeID {
	case 1:
		return "Boolean"
	case 3:
		return "Int16"
	case 4:
		return "UInt16"
	case 5:
		return "Int32"
	case 6:
		return "UInt32"
	case 7:
		return "Int64"
	case 8:
		return "UInt64"
	case 10:
		return "Float"
	case 11:
		return "Double"
	case 12:
		return "String"
	case 13:
		return "DateTime"
	case 16:
		return "ByteString"
	default:
		return fmt.Sprintf("Unknown(%d)", typeID)
	}
}

func (r *Reader) DiscoverNodes(ctx context.Context) error {
	nodes, err := r.BrowseAllNamespaces(ctx, "ns=0;i=85", 0)
	if err != nil {
		return err
	}
	r.nodeInfos = nodes
	r.logger.Info("Nodes discovered", "count", len(nodes))
	return nil
}

func (r *Reader) ReadDiscoveredNodes(ctx context.Context) ([]NodeInfo, error) {
	if len(r.nodeInfos) == 0 {
		r.logger.Warn("No nodes to read - nodeInfos is empty")
		return nil, nil
	}

	r.logger.Debug("Reading discovered nodes", "count", len(r.nodeInfos))

	const batchSize = 50
	var results []NodeInfo

	for i := 0; i < len(r.nodeInfos); i += batchSize {
		end := i + batchSize
		if end > len(r.nodeInfos) {
			end = len(r.nodeInfos)
		}

		batch := r.nodeInfos[i:end]
		nodeIDs := make([]*ua.ReadValueID, 0, len(batch))
		for j := range batch {
			nodeIDs = append(nodeIDs, &ua.ReadValueID{
				NodeID:       ua.MustParseNodeID(batch[j].ID),
				AttributeID: ua.AttributeIDValue,
			})
		}

		req := &ua.ReadRequest{
			MaxAge:     0,
			NodesToRead: nodeIDs,
		}

		resp, err := r.client.client.Read(ctx, req)
		if err != nil {
			r.logger.Error("Batch read failed", "error", err, "batch", i/batchSize)
			continue
		}

		for k, dr := range resp.Results {
			if k >= len(batch) {
				break
			}
			if dr.Status != ua.StatusOK {
				continue
			}
			if dr.Value != nil {
				batch[k].Value = dr.Value.Value()
				batch[k].Timestamp = time.Now()
			}
			results = append(results, batch[k])
		}
	}

	return results, nil
}
