package nxgo

import (
	"context"
	"fmt"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type CreateSheetParams struct {
	SheetName        string
	Units            string // "mm" or "inch"
	Height           float64
	Length           float64
	ScaleNumerator   float64
	ScaleDenominator float64
}

type DrawingSheet struct {
	session *Session
	part    *Part
	Ref     protocol.ObjectHandleWire
	Name    string
	Height  float64
	Length  float64
}

type ExportPDFParams struct {
	OutputPDFPath string
	SheetNames    []string
	ColorMode     string // "black_and_white", "color", "grayscale"
}

type ExportPDFResult struct {
	ExportedPath  string
	FileSizeBytes int64
}

func (p *Part) CreateDrawingSheet(ctx context.Context, params CreateSheetParams) (*DrawingSheet, error) {
	if params.Height == 0 {
		params.Height = 297.0 // A3 height mm
	}
	if params.Length == 0 {
		params.Length = 420.0 // A3 length mm
	}
	if params.ScaleNumerator == 0 {
		params.ScaleNumerator = 1.0
	}
	if params.ScaleDenominator == 0 {
		params.ScaleDenominator = 1.0
	}

	reqData, err := protocol.EncodePayload(protocol.DraftingCreateSheetRequest{
		PartRef:          &p.Ref,
		SheetName:        params.SheetName,
		Units:            params.Units,
		Height:           params.Height,
		Length:           params.Length,
		ScaleNumerator:   params.ScaleNumerator,
		ScaleDenominator: params.ScaleDenominator,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-sheet-%d", time.Now().UnixNano()),
		Op:        "drafting.create_sheet",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.DraftingCreateSheetResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &DrawingSheet{
		session: p.session,
		part:    p,
		Ref:     payload.SheetRef,
		Name:    payload.SheetName,
		Height:  payload.Height,
		Length:  payload.Length,
	}, nil
}

func (p *Part) ExportPDF(ctx context.Context, params ExportPDFParams) (*ExportPDFResult, error) {
	reqData, err := protocol.EncodePayload(protocol.DraftingExportPDFRequest{
		PartRef:       &p.Ref,
		OutputPDFPath: params.OutputPDFPath,
		SheetNames:    params.SheetNames,
		ColorMode:     params.ColorMode,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-pdf-%d", time.Now().UnixNano()),
		Op:        "drafting.export_pdf",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.DraftingExportPDFResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &ExportPDFResult{
		ExportedPath:  payload.ExportedPath,
		FileSizeBytes: payload.FileSizeBytes,
	}, nil
}

func (p *Part) DrawingSheets(ctx context.Context) ([]*DrawingSheet, error) {
	reqData, err := protocol.EncodePayload(protocol.DraftingQuerySheetsRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-qsheets-%d", time.Now().UnixNano()),
		Op:        "drafting.query_sheets",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.DraftingQuerySheetsResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	var sheets []*DrawingSheet
	for _, s := range payload.Sheets {
		sheets = append(sheets, &DrawingSheet{
			session: p.session,
			part:    p,
			Ref:     s.SheetRef,
			Name:    s.Name,
			Height:  s.Height,
			Length:  s.Length,
		})
	}
	return sheets, nil
}
