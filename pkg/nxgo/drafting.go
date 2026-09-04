package nxgo

import (
	"context"
	"fmt"
	"strings"

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
	if err := p.validate(); err != nil {
		return nil, err
	}
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
		RequestID: newRequestID("drafting.create_sheet"),
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
	if err := p.validate(); err != nil {
		return nil, err
	}
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
		RequestID: newRequestID("drafting.export_pdf"),
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
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.DraftingQuerySheetsRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("drafting.query_sheets"),
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

type StandardViewsParams struct {
	Layout             string  // "front_top_right_iso", "front_top_right", "front_top", "front_right"
	MarginBetweenViews float64 // mm, default 15
	MarginToBorder     float64 // mm, default 20
}

type StandardViewsResult struct {
	Created   bool
	Layout    string
	ViewCount int
	Views     []string
}

type AddNoteParams struct {
	TextLines []string
	OriginX   float64
	OriginY   float64
	Anchor    string  // "bottom_left", "bottom_right", "top_left", "top_right", "mid_center"
	TextSize  float64 // mm, default 3.5
}

type AddNoteResult struct {
	Added     bool
	LineCount int
	OriginX   float64
	OriginY   float64
}

func (p *Part) CreateStandardViews(ctx context.Context, params StandardViewsParams) (*StandardViewsResult, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.Layout == "" {
		params.Layout = "front_top_right_iso"
	}
	if params.MarginBetweenViews <= 0 {
		params.MarginBetweenViews = 15.0
	}
	if params.MarginToBorder <= 0 {
		params.MarginToBorder = 20.0
	}

	reqData, err := protocol.EncodePayload(protocol.DraftingCreateStandardViewsRequest{
		PartRef:            &p.Ref,
		Layout:             params.Layout,
		MarginBetweenViews: params.MarginBetweenViews,
		MarginToBorder:     params.MarginToBorder,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("drafting.create_standard_views"),
		Op:        "drafting.create_standard_views",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.DraftingCreateStandardViewsResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &StandardViewsResult{
		Created:   payload.Created,
		Layout:    payload.Layout,
		ViewCount: payload.ViewCount,
		Views:     payload.Views,
	}, nil
}

func (s *DrawingSheet) CreateStandardViews(ctx context.Context, params StandardViewsParams) (*StandardViewsResult, error) {
	return s.part.CreateStandardViews(ctx, params)
}

func (p *Part) AddNote(ctx context.Context, params AddNoteParams) (*AddNoteResult, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if len(params.TextLines) == 0 {
		return nil, nil
	}
	if params.TextSize <= 0 {
		params.TextSize = 3.5
	}
	if params.Anchor == "" {
		params.Anchor = "bottom_left"
	}

	reqData, err := protocol.EncodePayload(protocol.DraftingAddNoteRequest{
		PartRef:   &p.Ref,
		TextLines: params.TextLines,
		OriginX:   params.OriginX,
		OriginY:   params.OriginY,
		Anchor:    params.Anchor,
		TextSize:  params.TextSize,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("drafting.add_note"),
		Op:        "drafting.add_note",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.DraftingAddNoteResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &AddNoteResult{
		Added:     payload.Added,
		LineCount: payload.LineCount,
		OriginX:   payload.OriginX,
		OriginY:   payload.OriginY,
	}, nil
}

func (s *DrawingSheet) AddNote(ctx context.Context, params AddNoteParams) (*AddNoteResult, error) {
	return s.part.AddNote(ctx, params)
}

func (p *Part) AddBoundingDimensions(ctx context.Context, bbox *BoundingBox) (*AddNoteResult, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	var dimStr string
	if bbox != nil {
		dimStr = fmt.Sprintf("%.1f x %.1f x %.1f мм",
			bbox.Dimensions[0], bbox.Dimensions[1], bbox.Dimensions[2])
	} else {
		dimStr = "Габариты не определены"
	}

	lines := []string{
		"ТЕХНИЧЕСКИЕ ТРЕБОВАНИЯ И РАЗМЕРЫ:",
		"1. * Размеры для справок.",
		fmt.Sprintf("2. Габаритные размеры: %s.", dimStr),
		"3. Неуказанные предельные отклонения: H14, h14, ±IT14/2.",
		"4. Острые кромки притупить R 0.2...0.5 мм.",
	}

	return p.AddNote(ctx, AddNoteParams{
		TextLines: lines,
		OriginX:   25.0,
		OriginY:   15.0,
		Anchor:    "bottom_left",
		TextSize:  3.0,
	})
}

func (s *DrawingSheet) AddBoundingDimensions(ctx context.Context, bbox *BoundingBox) (*AddNoteResult, error) {
	return s.part.AddBoundingDimensions(ctx, bbox)
}

type AssemblyDrawingParams struct {
	SheetName        string
	SheetSize        string // "A3", "A2", "A1", "A4"
	Length           float64
	Height           float64
	Layout           string // "front_top_right_iso", "front_top_right", "isometric"
	IncludeBOMTable  bool
	IncludeTechNotes bool
	TechNotes        []string
}

func (p *Part) GenerateAssemblyDrawing(ctx context.Context, params AssemblyDrawingParams) (*DrawingSheet, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	length := params.Length
	height := params.Height
	if length <= 0 || height <= 0 {
		switch strings.ToUpper(params.SheetSize) {
		case "A4":
			length = 297.0
			height = 210.0
		case "A2":
			length = 594.0
			height = 420.0
		case "A1":
			length = 841.0
			height = 594.0
		default: // A3 default
			length = 420.0
			height = 297.0
		}
	}

	sheetName := params.SheetName
	if sheetName == "" {
		sheetName = "ASM_SHEET_1"
	}

	sheet, err := p.CreateDrawingSheet(ctx, CreateSheetParams{
		SheetName:        sheetName,
		Units:            "mm",
		Length:           length,
		Height:           height,
		ScaleNumerator:   1.0,
		ScaleDenominator: 1.0,
	})
	if err != nil {
		return nil, err
	}

	layout := params.Layout
	if layout == "" {
		layout = "front_top_right_iso"
	}
	_, _ = sheet.CreateStandardViews(ctx, StandardViewsParams{
		Layout:             layout,
		MarginBetweenViews: 20.0,
		MarginToBorder:     25.0,
	})

	if params.IncludeBOMTable {
		bomItems, err := p.BOM(ctx)
		if err == nil && len(bomItems) > 0 {
			bomLines := []string{
				"==================================================",
				"СПЕЦИФИКАЦИЯ СБОРОЧНЫХ ЕДИНИЦ И ДЕТАЛЕЙ (BOM)",
				"--------------------------------------------------",
				"Поз. | Наименование / Обозначение      | Кол-во",
				"--------------------------------------------------",
			}
			for i, item := range bomItems {
				name := item.PartName
				if name == "" {
					name = item.PartPath
				}
				bomLines = append(bomLines, fmt.Sprintf("%-4d | %-32s | %d шт", i+1, name, item.Quantity))
			}
			bomLines = append(bomLines, "==================================================")

			_, _ = sheet.AddNote(ctx, AddNoteParams{
				TextLines: bomLines,
				OriginX:   length - 210.0,
				OriginY:   height - float64(len(bomLines)*5) - 20.0,
				Anchor:    "top_left",
				TextSize:  2.8,
			})
		}
	}

	if params.IncludeTechNotes || len(params.TechNotes) > 0 {
		techLines := params.TechNotes
		if len(techLines) == 0 {
			techLines = []string{
				"ТЕХНИЧЕСКИЕ ТРЕБОВАНИЯ И СПЕЦИФИКАЦИЯ СБОРКИ:",
				"1. * Размеры для справок.",
				"2. Сборку производить в соответствии со сборочной схемой.",
				"3. Резьбовые соединения затянуть номинальным моментом.",
			}
		}
		_, _ = sheet.AddNote(ctx, AddNoteParams{
			TextLines: techLines,
			OriginX:   25.0,
			OriginY:   15.0,
			Anchor:    "bottom_left",
			TextSize:  3.0,
		})
	}

	return sheet, nil
}

