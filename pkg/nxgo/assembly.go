package nxgo

import (
	"context"
	"fmt"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type Matrix3D [9]float64

func IdentityMatrix() Matrix3D {
	return Matrix3D{1, 0, 0, 0, 1, 0, 0, 0, 1}
}

type AddComponentParams struct {
	PartPath      string
	ComponentName string
	Origin        Point3D
	Orientation   Matrix3D
	Layer         int
}

type Component struct {
	session  *Session
	part     *Part
	Ref      protocol.ObjectHandleWire
	Name     string
	PartPath string
}

type ComponentNode struct {
	Ref           protocol.ObjectHandleWire
	Name          string
	DisplayName   string
	PrototypePath string
	Position      Point3D
	Children      []ComponentNode
}

type BOMItem struct {
	PartName       string
	PartPath       string
	Quantity       int
	ComponentNames []string
}

func (p *Part) AddComponent(ctx context.Context, params AddComponentParams) (*Component, error) {
	if params.Orientation == (Matrix3D{}) {
		params.Orientation = IdentityMatrix()
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyAddComponentRequest{
		AssemblyPartRef: &p.Ref,
		PartPath:        params.PartPath,
		ComponentName:   params.ComponentName,
		Origin:          params.Origin,
		Orientation:     params.Orientation,
		Layer:           params.Layer,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-addcomp-%d", time.Now().UnixNano()),
		Op:        "assembly.add_component",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyAddComponentResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Component{
		session:  p.session,
		part:     p,
		Ref:      payload.ComponentRef,
		Name:     payload.ComponentName,
		PartPath: payload.PartPath,
	}, nil
}

func (p *Part) ComponentTree(ctx context.Context) (*ComponentNode, error) {
	reqData, err := protocol.EncodePayload(protocol.AssemblyQueryTreeRequest{
		AssemblyPartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-tree-%d", time.Now().UnixNano()),
		Op:        "assembly.query_tree",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyQueryTreeResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	root := convertNodeWire(payload.Root)
	return &root, nil
}

func convertNodeWire(w protocol.AssemblyComponentNodeWire) ComponentNode {
	node := ComponentNode{
		Ref:           w.ComponentRef,
		Name:          w.Name,
		DisplayName:   w.DisplayName,
		PrototypePath: w.PrototypePath,
		Position:      w.Position,
	}
	for _, ch := range w.Children {
		node.Children = append(node.Children, convertNodeWire(ch))
	}
	return node
}

func (p *Part) BOM(ctx context.Context) ([]BOMItem, error) {
	reqData, err := protocol.EncodePayload(protocol.AssemblyQueryBOMRequest{
		AssemblyPartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-bom-%d", time.Now().UnixNano()),
		Op:        "assembly.query_bom",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyQueryBOMResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	var items []BOMItem
	for _, item := range payload.Items {
		items = append(items, BOMItem{
			PartName:       item.PartName,
			PartPath:       item.PartPath,
			Quantity:       item.Quantity,
			ComponentNames: item.ComponentNames,
		})
	}
	return items, nil
}

func (c *Component) Remove(ctx context.Context) error {
	reqData, err := protocol.EncodePayload(protocol.AssemblyRemoveComponentRequest{
		AssemblyPartRef: &c.part.Ref,
		ComponentRef:    c.Ref,
	})
	if err != nil {
		return err
	}

	resp, err := c.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-remcomp-%d", time.Now().UnixNano()),
		Op:        "assembly.remove_component",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	return nil
}
