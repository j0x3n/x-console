package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// This file is the /responses side of the client (B39). The request is built
// as plain JSON: the SDK's union types add little for the few fields used.

// APIError is a non-2xx answer from /responses.
type APIError struct {
	URL        string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("POST %q: %d %s %s", e.URL, e.StatusCode, http.StatusText(e.StatusCode), e.Body)
}

func dataURL(img Image) string {
	return "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
}

func responsesBody(cfg Config, req Request, fallbackJSON, stream bool) (map[string]any, error) {
	instructions := req.System
	if fallbackJSON {
		instructions += "\n只输出 JSON，格式必须符合以下 JSON Schema：" + string(req.JSONSchema)
	}
	input := []any{}
	for _, message := range req.Messages {
		switch message.Role {
		case "user":
			parts := []any{map[string]any{"type": "input_text", "text": message.Content}}
			for _, img := range message.Images {
				parts = append(parts, map[string]any{"type": "input_image", "image_url": dataURL(img)})
			}
			input = append(input, map[string]any{"role": "user", "content": parts})
		case "assistant":
			if message.Content != "" {
				input = append(input, map[string]any{"role": "assistant", "content": message.Content})
			}
			for _, call := range message.ToolCalls {
				args := string(call.Arguments)
				if args == "" {
					args = "{}"
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": args})
			}
		case "tool":
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Content})
		default:
			return nil, fmt.Errorf("unsupported message role: %s", message.Role)
		}
	}
	body := map[string]any{"model": cfg.Model, "input": input, "store": false, "stream": stream}
	if instructions != "" {
		body["instructions"] = instructions
	}
	if len(req.Tools) > 0 {
		tools := []any{}
		for _, tool := range req.Tools {
			var schema any
			if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
				return nil, err
			}
			tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": schema, "strict": false})
		}
		body["tools"] = tools
	}
	if cfg.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": cfg.ReasoningEffort}
	}
	if len(req.JSONSchema) > 0 && !fallbackJSON {
		var schema any
		if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
			return nil, err
		}
		body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "result", "schema": schema}}
	}
	return body, nil
}

// postResponses sends the request and returns the open response on 2xx.
func postResponses(ctx context.Context, cfg Config, body map[string]any, stream bool) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/responses"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	key := cfg.APIKey
	if key == "" {
		key = "local-model"
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		return nil, &APIError{URL: url, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(detail))}
	}
	return resp, nil
}

type responseObject struct {
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"output"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r responseObject) result() Result {
	var out Result
	for _, item := range r.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					out.Text += part.Text
				}
			}
		case "function_call":
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: item.CallID, Name: item.Name, Arguments: json.RawMessage(item.Arguments)})
		}
	}
	out.InputTokens, out.OutputTokens = r.Usage.InputTokens, r.Usage.OutputTokens
	out.Truncated = r.Status == "incomplete" && r.IncompleteDetails != nil && r.IncompleteDetails.Reason == "max_output_tokens"
	return out
}

// retryResponses runs call and, like the chat side, drops the reasoning
// setting or the JSON schema format when the endpoint says it does not
// support them.
func (s *service) retryResponses(ctx context.Context, cfg Config, req Request, stream bool) (*http.Response, Config, bool, error) {
	fallbackJSON := false
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var body map[string]any
		body, err = responsesBody(cfg, req, fallbackJSON, stream)
		if err != nil {
			return nil, cfg, fallbackJSON, err
		}
		var resp *http.Response
		resp, err = postResponses(ctx, cfg, body, stream)
		if err == nil {
			return resp, cfg, fallbackJSON, nil
		}
		if cfg.ReasoningEffort != "" && unsupported(err, "reasoning") {
			cfg.ReasoningEffort = ""
			if s.unsupported != nil {
				_ = s.unsupported(context.WithoutCancel(ctx), cfg.ProviderID)
			}
			continue
		}
		if len(req.JSONSchema) > 0 && !fallbackJSON && (unsupported(err, "text.format") || unsupported(err, "json_schema")) {
			fallbackJSON = true
			continue
		}
		return nil, cfg, fallbackJSON, err
	}
	return nil, cfg, fallbackJSON, err
}

func (s *service) completeResponses(ctx context.Context, cfg Config, req Request) (Result, error) {
	resp, _, fallbackJSON, err := s.retryResponses(ctx, cfg, req, false)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	var object responseObject
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&object); err != nil {
		return Result{}, fmt.Errorf("responses 结果解析失败: %w", err)
	}
	if object.Error != nil && object.Error.Message != "" {
		return Result{}, errors.New(object.Error.Message)
	}
	result := object.result()
	if fallbackJSON {
		result.Text = extractJSON(result.Text)
	}
	return result, nil
}

type responsesStream struct {
	body         io.ReadCloser
	scanner      *bufio.Scanner
	current      Delta
	result       Result
	final        *Result
	err          error
	done         bool
	fallbackJSON bool
	config       Config
	purpose      string
	start        time.Time
	record       RecordFunc
	ctx          context.Context
	recorded     bool
}

func (s *service) streamResponses(ctx context.Context, cfg Config, req Request) (Stream, error) {
	start := time.Now()
	resp, cfg, fallbackJSON, err := s.retryResponses(ctx, cfg, req, true)
	if err != nil {
		if s.record != nil {
			s.record(context.WithoutCancel(ctx), cfg, req.Purpose, Result{}, time.Since(start), err)
		}
		return nil, err
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	return &responsesStream{body: resp.Body, scanner: scanner, fallbackJSON: fallbackJSON, config: cfg, purpose: req.Purpose,
		start: start, record: s.record, ctx: ctx}, nil
}

// Next reads server-sent events until one carries text or the stream ends.
func (s *responsesStream) Next() bool {
	s.current = Delta{}
	for !s.done && s.scanner.Scan() {
		line := s.scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Message  string          `json:"message"`
			Item     json.RawMessage `json:"item"`
			Response *responseObject `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			s.result.Text += event.Delta
			s.current.Text = event.Delta
			return true
		case "response.output_item.done":
			var item struct {
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(event.Item, &item) == nil && item.Type == "function_call" {
				s.result.ToolCalls = append(s.result.ToolCalls, ToolCall{ID: item.CallID, Name: item.Name, Arguments: json.RawMessage(item.Arguments)})
			}
		case "response.completed", "response.incomplete":
			if event.Response != nil {
				final := event.Response.result()
				s.final = &final
			}
			s.done = true
		case "response.failed":
			message := "模型调用失败"
			if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
				message = event.Response.Error.Message
			}
			s.err, s.done = errors.New(message), true
		case "error":
			s.err, s.done = errors.New(event.Message), true
		}
	}
	if err := s.scanner.Err(); err != nil && s.err == nil {
		s.err = err
	}
	if !s.done && s.err == nil {
		s.err = io.ErrUnexpectedEOF // the stream ended without response.completed
	}
	s.done = true
	return false
}

func (s *responsesStream) Current() Delta { return s.current }

func (s *responsesStream) Result() Result {
	result := s.result
	if s.final != nil {
		// The final response is complete; the deltas may miss parts.
		if s.final.Text != "" {
			result.Text = s.final.Text
		}
		if len(s.final.ToolCalls) > 0 {
			result.ToolCalls = s.final.ToolCalls
		}
		result.InputTokens, result.OutputTokens, result.Truncated = s.final.InputTokens, s.final.OutputTokens, s.final.Truncated
	}
	if s.fallbackJSON {
		result.Text = extractJSON(result.Text)
	}
	return result
}

func (s *responsesStream) Err() error { return s.err }

func (s *responsesStream) Close() error {
	err := s.body.Close()
	if !s.recorded && s.record != nil {
		s.recorded = true
		s.record(context.WithoutCancel(s.ctx), s.config, s.purpose, s.Result(), time.Since(s.start), s.err)
	}
	return err
}
