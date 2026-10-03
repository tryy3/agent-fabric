package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/scrub"
)

// HopCapture is a scrubbed inter-service traffic record for the Inspector.
type HopCapture struct {
	ID         string         `json:"id"`
	ThreadID   string         `json:"threadId"`
	MessageID  *string        `json:"messageId,omitempty"`
	SessionID  *string        `json:"sessionId,omitempty"`
	RoundIndex int            `json:"roundIndex"`
	HopKind    string         `json:"hopKind"`
	Direction  string         `json:"direction"`
	Method     *string        `json:"method,omitempty"`
	URL        *string        `json:"url,omitempty"`
	StatusCode *int           `json:"statusCode,omitempty"`
	Headers    map[string]any `json:"headers"`
	BodyText   string         `json:"bodyText"`
	Meta       map[string]any `json:"meta"`
	CreatedAt  time.Time      `json:"createdAt"`
}

// InsertHTTPHopCaptureParams is input for a non-LLM HTTP/MCP hop capture.
type InsertHTTPHopCaptureParams struct {
	ThreadID   string
	SessionID  string
	RoundIndex int
	HopKind    string // "http" or "mcp"
	Method     string
	URL        string
	StatusCode int
	ReqBody    string
	RespBody   string
	Meta       map[string]any
	Pipeline   scrub.Pipeline
}

// InsertHTTPHopCapture scrubs and persists one HTTP or MCP hop capture.
func (s *Store) InsertHTTPHopCapture(ctx context.Context, p InsertHTTPHopCaptureParams) (HopCapture, error) {
	pipe := p.Pipeline
	if pipe.Headers == nil {
		pipe.Headers = scrub.DefaultHeaders{}
	}
	if pipe.Body == nil {
		pipe.Body = scrub.Identity{}
	}
	reqBody := pipe.ScrubBody(ctx, p.ReqBody)
	respBody := pipe.ScrubBody(ctx, p.RespBody)

	headersObj := map[string]any{
		"request":  map[string]any{},
		"response": map[string]any{},
	}
	headersJSON, err := json.Marshal(headersObj)
	if err != nil {
		return HopCapture{}, fmt.Errorf("marshal headers: %w", err)
	}

	meta := map[string]any{}
	for k, v := range p.Meta {
		meta[k] = v
	}
	meta["response_body"] = respBody
	meta["scrubber"] = "prompt-scrub+headers"
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return HopCapture{}, fmt.Errorf("marshal meta: %w", err)
	}

	hopKind := p.HopKind
	if hopKind == "" {
		hopKind = "http"
	}
	id, err := newID("cap_")
	if err != nil {
		return HopCapture{}, err
	}
	sessionID := p.SessionID
	method := p.Method
	url := p.URL
	status := int32(p.StatusCode)
	row, err := s.q.InsertHopCapture(ctx, db.InsertHopCaptureParams{
		ID:          id,
		ThreadID:    p.ThreadID,
		MessageID:   nil,
		SessionID:   &sessionID,
		RoundIndex:  int32(p.RoundIndex),
		HopKind:     hopKind,
		Direction:   "exchange",
		Method:      &method,
		Url:         &url,
		StatusCode:  &status,
		HeadersJson: headersJSON,
		BodyText:    reqBody,
		MetaJson:    metaJSON,
		CreatedAt:   timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		return HopCapture{}, fmt.Errorf("insert hop capture: %w", err)
	}
	return hopCaptureFromDB(row)
}

// InsertLLMHopCaptureParams is the scrubbed-ready input for an LLM exchange capture.
type InsertLLMHopCaptureParams struct {
	ThreadID    string
	SessionID   string
	RoundIndex  int
	Method      string
	URL         string
	StatusCode  int
	ReqHeaders  http.Header
	RespHeaders http.Header
	ReqBody     string
	RespBody    string
	Meta        map[string]any
	Pipeline    scrub.Pipeline
}

// InsertLLMHopCapture scrubs and persists one LLM HTTP exchange (message_id null until CommitTurn).
func (s *Store) InsertLLMHopCapture(ctx context.Context, p InsertLLMHopCaptureParams) (HopCapture, error) {
	pipe := p.Pipeline
	if pipe.Headers == nil {
		pipe.Headers = scrub.DefaultHeaders{}
	}
	if pipe.Body == nil {
		pipe.Body = scrub.Identity{}
	}

	reqH := pipe.RedactHeaders(p.ReqHeaders)
	respH := pipe.RedactHeaders(p.RespHeaders)
	reqBody := pipe.ScrubBody(ctx, p.ReqBody)
	respBody := pipe.ScrubBody(ctx, p.RespBody)

	headersObj := map[string]any{
		"request":  headerToMap(reqH),
		"response": headerToMap(respH),
	}
	headersJSON, err := json.Marshal(headersObj)
	if err != nil {
		return HopCapture{}, fmt.Errorf("marshal headers: %w", err)
	}

	meta := map[string]any{}
	for k, v := range p.Meta {
		meta[k] = v
	}
	meta["response_body"] = respBody
	meta["scrubber"] = "prompt-scrub+headers"
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return HopCapture{}, fmt.Errorf("marshal meta: %w", err)
	}

	id, err := newID("cap_")
	if err != nil {
		return HopCapture{}, err
	}
	sessionID := p.SessionID
	method := p.Method
	url := p.URL
	status := int32(p.StatusCode)
	row, err := s.q.InsertHopCapture(ctx, db.InsertHopCaptureParams{
		ID:          id,
		ThreadID:    p.ThreadID,
		MessageID:   nil,
		SessionID:   &sessionID,
		RoundIndex:  int32(p.RoundIndex),
		HopKind:     "llm",
		Direction:   "exchange",
		Method:      &method,
		Url:         &url,
		StatusCode:  &status,
		HeadersJson: headersJSON,
		BodyText:    reqBody,
		MetaJson:    metaJSON,
		CreatedAt:   timestamptzFromTime(time.Now().UTC()),
	})
	if err != nil {
		return HopCapture{}, fmt.Errorf("insert hop capture: %w", err)
	}
	return hopCaptureFromDB(row)
}

// LinkHopCapturesToMessage attaches in-flight session captures to an assistant message.
func (s *Store) LinkHopCapturesToMessage(ctx context.Context, threadID, sessionID, messageID string) error {
	if err := s.q.LinkHopCapturesToMessage(ctx, db.LinkHopCapturesToMessageParams{
		ThreadID:  threadID,
		SessionID: &sessionID,
		MessageID: &messageID,
	}); err != nil {
		return fmt.Errorf("link hop captures: %w", err)
	}
	return nil
}

// DeleteUnlinkedHopCaptures removes in-flight captures for a cancelled/failed turn.
func (s *Store) DeleteUnlinkedHopCaptures(ctx context.Context, threadID, sessionID string) error {
	if err := s.q.DeleteHopCapturesBySession(ctx, db.DeleteHopCapturesBySessionParams{
		ThreadID:  threadID,
		SessionID: &sessionID,
	}); err != nil {
		return fmt.Errorf("delete hop captures: %w", err)
	}
	return nil
}

// ListHopCapturesByMessage returns scrubbed captures for an assistant message.
func (s *Store) ListHopCapturesByMessage(ctx context.Context, threadID, messageID string) ([]HopCapture, error) {
	rows, err := s.q.ListHopCapturesByMessage(ctx, db.ListHopCapturesByMessageParams{
		ThreadID:  threadID,
		MessageID: &messageID,
	})
	if err != nil {
		return nil, fmt.Errorf("list hop captures: %w", err)
	}
	out := make([]HopCapture, 0, len(rows))
	for _, row := range rows {
		c, err := hopCaptureFromDB(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ListHopCapturesByThread returns scrubbed captures for all committed turns in a thread.
func (s *Store) ListHopCapturesByThread(ctx context.Context, threadID string) ([]HopCapture, error) {
	rows, err := s.q.ListHopCapturesByThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("list hop captures by thread: %w", err)
	}
	out := make([]HopCapture, 0, len(rows))
	for _, row := range rows {
		c, err := hopCaptureFromDB(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func headerToMap(h http.Header) map[string]any {
	out := make(map[string]any, len(h))
	for k, vals := range h {
		if len(vals) == 1 {
			out[k] = vals[0]
		} else {
			out[k] = vals
		}
	}
	return out
}

func hopCaptureFromDB(row db.HopCapture) (HopCapture, error) {
	var headers map[string]any
	if err := json.Unmarshal(row.HeadersJson, &headers); err != nil {
		return HopCapture{}, fmt.Errorf("decode headers: %w", err)
	}
	var meta map[string]any
	if len(row.MetaJson) == 0 {
		meta = map[string]any{}
	} else if err := json.Unmarshal(row.MetaJson, &meta); err != nil {
		return HopCapture{}, fmt.Errorf("decode meta: %w", err)
	}
	var status *int
	if row.StatusCode != nil {
		v := int(*row.StatusCode)
		status = &v
	}
	return HopCapture{
		ID:         row.ID,
		ThreadID:   row.ThreadID,
		MessageID:  row.MessageID,
		SessionID:  row.SessionID,
		RoundIndex: int(row.RoundIndex),
		HopKind:    row.HopKind,
		Direction:  row.Direction,
		Method:     row.Method,
		URL:        row.Url,
		StatusCode: status,
		Headers:    headers,
		BodyText:   row.BodyText,
		Meta:       meta,
		CreatedAt:  timeFromTimestamptz(row.CreatedAt),
	}, nil
}
