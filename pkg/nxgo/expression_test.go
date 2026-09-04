package nxgo

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func setupTestSessionWithResponder(t *testing.T, responder func(op string, reqPayload []byte) ([]byte, error)) (*Session, func()) {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	serverFramed := pipe.NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)

	go func() {
		for {
			rawReq, err := serverFramed.Receive()
			if err != nil {
				return
			}
			req, err := protocol.DecodePayload[protocol.RequestEnvelope](rawReq)
			if err != nil {
				return
			}

			var resp protocol.ResponseEnvelope
			resp.RequestID = req.RequestID

			payloadBytes, err := responder(req.Op, req.Payload)
			if err != nil {
				resp.Status = protocol.StatusError
				resp.Error = &protocol.ErrorEnvelope{
					Category: "OperationFailed",
					Message:  err.Error(),
					Op:       req.Op,
				}
			} else {
				resp.Status = protocol.StatusOK
				resp.Payload = payloadBytes
			}

			respBytes, err := protocol.EncodePayload(resp)
			if err != nil {
				return
			}
			if err := serverFramed.Send(respBytes); err != nil {
				return
			}
		}
	}()

	client := pipe.NewClient(clientConn)
	session := WrapClient(client, "test-session", 1, "v2512")

	cleanup := func() {
		_ = client.Close()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}

	return session, cleanup
}

func TestExpressionCreateAndQuery(t *testing.T) {
	expressions := make(map[string]protocol.ExpressionInfoWire)

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "expression.create":
			req, err := protocol.DecodePayload[protocol.ExpressionCreateRequest](payload)
			if err != nil {
				return nil, err
			}
			info := protocol.ExpressionInfoWire{
				ExpressionRef: protocol.ObjectHandleWire{
					SessionID:  "test-session",
					Epoch:      1,
					ObjectID:   "expr-" + req.Name,
					Generation: 1,
					Kind:       "Expression",
				},
				Name:        req.Name,
				Formula:     req.Formula,
				Value:       100.0,
				StringValue: "100.0",
				Type:        req.Type,
				Units:       req.Units,
				NativeTag:   12345,
			}
			expressions[req.Name] = info
			return protocol.EncodePayload(protocol.ExpressionCreateResponse{
				ExpressionRef: info.ExpressionRef,
				Name:          info.Name,
				Formula:       info.Formula,
				Value:         info.Value,
				StringValue:   info.StringValue,
				Type:          info.Type,
				Units:         info.Units,
				NativeTag:     info.NativeTag,
			})

		case "expression.query":
			req, err := protocol.DecodePayload[protocol.ExpressionQueryRequest](payload)
			if err != nil {
				return nil, err
			}
			var list []protocol.ExpressionInfoWire
			if req.Name != "" {
				if expr, ok := expressions[req.Name]; ok {
					list = append(list, expr)
				}
			} else {
				for _, expr := range expressions {
					list = append(list, expr)
				}
			}
			return protocol.EncodePayload(protocol.ExpressionQueryResponse{
				Expressions: list,
			})

		default:
			return nil, errors.New("unsupported op: " + op)
		}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
		Name:  "test_part",
		Units: "mm",
	}

	// 1. Create expression with units
	expr1, err := part.CreateExpression(ctx, "length", "100.0", "mm")
	if err != nil {
		t.Fatalf("CreateExpression failed: %v", err)
	}
	if expr1.Name != "length" || expr1.Formula != "100.0" || expr1.Units != "mm" {
		t.Errorf("unexpected expr1: %+v", expr1)
	}
	if expr1.Value != 100.0 {
		t.Errorf("expected value 100.0, got %f", expr1.Value)
	}

	// 2. Create expression with params
	expr2, err := part.CreateExpressionWithParams(ctx, ExpressionParams{
		Name:    "width",
		Formula: "length * 0.5",
		Units:   "mm",
	})
	if err != nil {
		t.Fatalf("CreateExpressionWithParams failed: %v", err)
	}
	if expr2.Name != "width" {
		t.Errorf("expected name width, got %s", expr2.Name)
	}

	// 3. Query single expression
	found, err := part.GetExpression(ctx, "length")
	if err != nil {
		t.Fatalf("GetExpression failed: %v", err)
	}
	if found.Name != "length" || found.Value != 100.0 {
		t.Errorf("GetExpression mismatch: %+v", found)
	}

	// 4. Query non-existent expression
	_, err = part.GetExpression(ctx, "nonexistent")
	if !errors.Is(err, ErrExpressionNotFound) {
		t.Errorf("expected ErrExpressionNotFound, got %v", err)
	}

	// 5. List expressions
	all, err := part.ListExpressions(ctx)
	if err != nil {
		t.Fatalf("ListExpressions failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 expressions, got %d", len(all))
	}
}

func TestExpressionEditAndDelete(t *testing.T) {
	expressions := map[string]protocol.ExpressionInfoWire{
		"height": {
			ExpressionRef: protocol.ObjectHandleWire{
				SessionID:  "test-session",
				Epoch:      1,
				ObjectID:   "expr-height",
				Generation: 1,
				Kind:       "Expression",
			},
			Name:        "height",
			Formula:     "50.0",
			Value:       50.0,
			StringValue: "50.0",
			Type:        "Number",
			Units:       "mm",
			NativeTag:   54321,
		},
	}

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "expression.edit":
			req, err := protocol.DecodePayload[protocol.ExpressionEditRequest](payload)
			if err != nil {
				return nil, err
			}
			info, ok := expressions[req.Name]
			if !ok {
				return nil, errors.New("expression not found: " + req.Name)
			}
			info.Formula = req.Formula
			info.Value = 75.0
			info.StringValue = "75.0"
			expressions[req.Name] = info
			return protocol.EncodePayload(protocol.ExpressionEditResponse{
				ExpressionRef: info.ExpressionRef,
				Name:          info.Name,
				Formula:       info.Formula,
				Value:         info.Value,
				StringValue:   info.StringValue,
				Updated:       true,
			})

		case "expression.delete":
			req, err := protocol.DecodePayload[protocol.ExpressionDeleteRequest](payload)
			if err != nil {
				return nil, err
			}
			if _, ok := expressions[req.Name]; !ok {
				return protocol.EncodePayload(protocol.ExpressionDeleteResponse{Deleted: false})
			}
			delete(expressions, req.Name)
			return protocol.EncodePayload(protocol.ExpressionDeleteResponse{Deleted: true})

		default:
			return nil, errors.New("unsupported op: " + op)
		}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
		Name:  "test_part",
		Units: "mm",
	}

	// 1. Edit expression via Part.SetExpression
	updated, err := part.SetExpression(ctx, "height", "75.0")
	if err != nil {
		t.Fatalf("SetExpression failed: %v", err)
	}
	if updated.Formula != "75.0" || updated.Value != 75.0 {
		t.Errorf("SetExpression mismatch: %+v", updated)
	}

	// 2. Edit expression via Expression.Edit
	err = updated.Edit(ctx, "80.0")
	if err != nil {
		t.Fatalf("Expression.Edit failed: %v", err)
	}
	if updated.Formula != "80.0" {
		t.Errorf("expected formula 80.0, got %s", updated.Formula)
	}

	// 3. Delete expression via Expression.Delete
	err = updated.Delete(ctx)
	if err != nil {
		t.Fatalf("Expression.Delete failed: %v", err)
	}

	// 4. Delete non-existent expression returns error
	err = part.DeleteExpression(ctx, "nonexistent")
	if err == nil {
		t.Errorf("expected error deleting nonexistent expression, got nil")
	}
}

func TestExpressionValidation(t *testing.T) {
	part := &Part{
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
	}
	ctx := context.Background()

	// Empty name
	_, err := part.CreateExpression(ctx, "", "100.0", "mm")
	if !errors.Is(err, ErrEmptyExpressionName) {
		t.Errorf("expected ErrEmptyExpressionName, got %v", err)
	}

	// Empty formula
	_, err = part.CreateExpression(ctx, "p1", "", "mm")
	if !errors.Is(err, ErrEmptyExpressionFormula) {
		t.Errorf("expected ErrEmptyExpressionFormula, got %v", err)
	}

	// Get with empty name
	_, err = part.GetExpression(ctx, "")
	if !errors.Is(err, ErrEmptyExpressionName) {
		t.Errorf("expected ErrEmptyExpressionName, got %v", err)
	}

	// Set with empty name
	_, err = part.SetExpression(ctx, "", "100.0")
	if !errors.Is(err, ErrEmptyExpressionName) {
		t.Errorf("expected ErrEmptyExpressionName, got %v", err)
	}

	// Delete with empty name
	err = part.DeleteExpression(ctx, "")
	if !errors.Is(err, ErrEmptyExpressionName) {
		t.Errorf("expected ErrEmptyExpressionName, got %v", err)
	}

	// Expression method with unbound part
	orphan := &Expression{Name: "orphan"}
	if err := orphan.Edit(ctx, "100"); err == nil {
		t.Errorf("expected error on unbound expression edit, got nil")
	}
	if err := orphan.Delete(ctx); err == nil {
		t.Errorf("expected error on unbound expression delete, got nil")
	}
}
