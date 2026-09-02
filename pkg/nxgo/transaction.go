package nxgo

import (
	"context"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type Transaction struct {
	session *Session
	TxID    string
	MarkID  int
}

func (s *Session) BeginTx(ctx context.Context, name string) (*Transaction, error) {
	reqData, err := protocol.EncodePayload(protocol.TransactionBeginRequest{Name: name})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("transaction.begin"),
		Op:        "transaction.begin",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.TransactionBeginResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		session: s,
		TxID:    payload.TxID,
		MarkID:  payload.MarkID,
	}, nil
}

func (tx *Transaction) Commit(ctx context.Context) error {
	reqData, err := protocol.EncodePayload(protocol.TransactionCommitRequest{TxID: tx.TxID})
	if err != nil {
		return err
	}

	resp, err := tx.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("transaction.commit"),
		Op:        "transaction.commit",
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

func (tx *Transaction) Rollback(ctx context.Context) error {
	reqData, err := protocol.EncodePayload(protocol.TransactionRollbackRequest{TxID: tx.TxID})
	if err != nil {
		return err
	}

	resp, err := tx.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("transaction.rollback"),
		Op:        "transaction.rollback",
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
