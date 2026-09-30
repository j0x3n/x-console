// Package llm is the OpenAI compatible Chat Completions boundary for AI features.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

var ErrNotConfigured = errors.New("AI 模型还没配置")

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type Request struct {
	Purpose    string
	System     string
	Messages   []Message
	Tools      []Tool
	JSONSchema json.RawMessage
}

type Result struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int64
	OutputTokens int64
	// Truncated is set when the model stopped at its output limit
	// (finish_reason "length"). Tool call arguments may then be cut off.
	Truncated bool
}

type ToolCallDelta struct {
	Index     int64
	ID        string
	Name      string
	Arguments string
}

type Delta struct {
	Text      string
	ToolCalls []ToolCallDelta
}

type Stream interface {
	Next() bool
	Current() Delta
	Result() Result
	Err() error
	Close() error
}

type Client interface {
	Stream(context.Context, Request) (Stream, error)
	Complete(context.Context, Request) (Result, error)
}

type Config struct {
	ProviderID      int64
	ProviderName    string
	BaseURL         string
	APIKey          string
	Model           string
	ReasoningEffort string
	InputPrice      *float32
	OutputPrice     *float32
}

type ResolveFunc func(context.Context, string) (Config, error)
type RecordFunc func(context.Context, Config, string, Result, time.Duration, error)
type UnsupportedFunc func(context.Context, int64) error

type service struct {
	resolve     ResolveFunc
	record      RecordFunc
	unsupported UnsupportedFunc
}

func New(resolve ResolveFunc, record RecordFunc, unsupported UnsupportedFunc) Client {
	return &service{resolve: resolve, record: record, unsupported: unsupported}
}

func requestParams(cfg Config, req Request, fallbackJSON bool) (openai.ChatCompletionNewParams, error) {
	params := openai.ChatCompletionNewParams{Model: openai.ChatModel(cfg.Model)}
	if req.System != "" {
		params.Messages = append(params.Messages, openai.SystemMessage(req.System))
	}
	if fallbackJSON {
		params.Messages = append(params.Messages, openai.SystemMessage("只输出 JSON，格式必须符合以下 JSON Schema："+string(req.JSONSchema)))
	}
	for _, message := range req.Messages {
		switch message.Role {
		case "user":
			params.Messages = append(params.Messages, openai.UserMessage(message.Content))
		case "assistant":
			assistant := openai.AssistantMessage(message.Content)
			for _, call := range message.ToolCalls {
				assistant.OfAssistant.ToolCalls = append(assistant.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{ID: call.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: call.Name, Arguments: string(call.Arguments)}},
				})
			}
			params.Messages = append(params.Messages, assistant)
		case "tool":
			params.Messages = append(params.Messages, openai.ToolMessage(message.Content, message.ToolCallID))
		default:
			return params, fmt.Errorf("unsupported message role: %s", message.Role)
		}
	}
	for _, tool := range req.Tools {
		var schema shared.FunctionParameters
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			return params, err
		}
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: tool.Name, Description: param.NewOpt(tool.Description), Parameters: schema}))
	}
	if cfg.ReasoningEffort != "" && req.Purpose == "agent" {
		params.ReasoningEffort = shared.ReasoningEffort(cfg.ReasoningEffort)
	}
	if len(req.JSONSchema) > 0 && !fallbackJSON {
		var schema any
		if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
			return params, err
		}
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{Name: "result", Schema: schema}}}
	}
	return params, nil
}

func sdk(cfg Config) openai.Client {
	key := cfg.APIKey
	if key == "" {
		key = "local-model"
	}
	return openai.NewClient(option.WithBaseURL(cfg.BaseURL), option.WithAPIKey(key))
}

func unsupported(err error, field string) bool {
	var apiErr *openai.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == 400 && strings.Contains(strings.ToLower(apiErr.Error()), field)
}

func (s *service) Complete(ctx context.Context, req Request) (result Result, err error) {
	cfg, err := s.resolve(ctx, req.Purpose)
	if err != nil {
		return result, err
	}
	start := time.Now()
	defer func() {
		if s.record != nil {
			s.record(context.WithoutCancel(ctx), cfg, req.Purpose, result, time.Since(start), err)
		}
	}()
	client := sdk(cfg)
	fallbackJSON := false
	for attempt := 0; attempt < 3; attempt++ {
		var params openai.ChatCompletionNewParams
		params, err = requestParams(cfg, req, fallbackJSON)
		if err != nil {
			return result, err
		}
		var response *openai.ChatCompletion
		response, err = client.Chat.Completions.New(ctx, params)
		if err == nil {
			if len(response.Choices) > 0 {
				result.Text = response.Choices[0].Message.Content
				result.Truncated = response.Choices[0].FinishReason == "length"
				for _, call := range response.Choices[0].Message.ToolCalls {
					if call.Type == "function" {
						result.ToolCalls = append(result.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
					}
				}
			}
			result.InputTokens, result.OutputTokens = response.Usage.PromptTokens, response.Usage.CompletionTokens
			if fallbackJSON {
				result.Text = extractJSON(result.Text)
			}
			return result, nil
		}
		if cfg.ReasoningEffort != "" && unsupported(err, "reasoning") {
			cfg.ReasoningEffort = ""
			if s.unsupported != nil {
				_ = s.unsupported(context.WithoutCancel(ctx), cfg.ProviderID)
			}
			continue
		}
		if len(req.JSONSchema) > 0 && !fallbackJSON && unsupported(err, "response_format") {
			fallbackJSON = true
			continue
		}
		return result, err
	}
	return result, err
}

func extractJSON(text string) string {
	first, last := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
	if first >= 0 && last >= first {
		return text[first : last+1]
	}
	return text
}

type stream struct {
	inner interface {
		Next() bool
		Current() openai.ChatCompletionChunk
		Err() error
		Close() error
	}
	buffered     *openai.ChatCompletionChunk
	current      Delta
	result       Result
	tool         map[int64]*ToolCall
	fallbackJSON bool
	config       Config
	purpose      string
	start        time.Time
	record       RecordFunc
	ctx          context.Context
	once         sync.Once
}

func (s *service) Stream(ctx context.Context, req Request) (Stream, error) {
	cfg, err := s.resolve(ctx, req.Purpose)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	client := sdk(cfg)
	var inner *stream
	fallbackJSON := false
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		params, err := requestParams(cfg, req, fallbackJSON)
		if err != nil {
			if s.record != nil {
				s.record(context.WithoutCancel(ctx), cfg, req.Purpose, Result{}, time.Since(start), err)
			}
			return nil, err
		}
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: param.NewOpt(true)}
		upstream := client.Chat.Completions.NewStreaming(ctx, params)
		inner = &stream{inner: upstream, tool: map[int64]*ToolCall{}, config: cfg, purpose: req.Purpose, start: start, record: s.record, ctx: ctx, fallbackJSON: fallbackJSON}
		if upstream.Next() {
			chunk := upstream.Current()
			inner.buffered = &chunk
			return inner, nil
		}
		err = upstream.Err()
		_ = upstream.Close()
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		lastErr = err
		if cfg.ReasoningEffort != "" && unsupported(err, "reasoning") {
			cfg.ReasoningEffort = ""
			if s.unsupported != nil {
				_ = s.unsupported(context.WithoutCancel(ctx), cfg.ProviderID)
			}
			continue
		}
		if len(req.JSONSchema) > 0 && !fallbackJSON && unsupported(err, "response_format") {
			fallbackJSON = true
			continue
		}
		if s.record != nil {
			s.record(context.WithoutCancel(ctx), cfg, req.Purpose, Result{}, time.Since(start), err)
		}
		return nil, err
	}
	if s.record != nil {
		s.record(context.WithoutCancel(ctx), cfg, req.Purpose, Result{}, time.Since(start), lastErr)
	}
	return nil, lastErr
}

func (s *stream) Next() bool {
	var chunk openai.ChatCompletionChunk
	if s.buffered != nil {
		chunk, s.buffered = *s.buffered, nil
	} else {
		if !s.inner.Next() {
			return false
		}
		chunk = s.inner.Current()
	}
	s.current = Delta{}
	if chunk.JSON.Usage.Valid() {
		s.result.InputTokens, s.result.OutputTokens = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
	}
	for _, choice := range chunk.Choices {
		if choice.FinishReason == "length" {
			s.result.Truncated = true
		}
		s.current.Text += choice.Delta.Content
		s.result.Text += choice.Delta.Content
		for _, call := range choice.Delta.ToolCalls {
			current := s.tool[call.Index]
			if current == nil {
				current = &ToolCall{}
				s.tool[call.Index] = current
			}
			current.ID += call.ID
			current.Name += call.Function.Name
			current.Arguments = append(current.Arguments, call.Function.Arguments...)
			s.current.ToolCalls = append(s.current.ToolCalls, ToolCallDelta{Index: call.Index, ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
		}
	}
	return true
}

func (s *stream) Current() Delta { return s.current }
func (s *stream) Result() Result {
	result := s.result
	if s.fallbackJSON {
		result.Text = extractJSON(result.Text)
	}
	indices := make([]int64, 0, len(s.tool))
	for index := range s.tool {
		indices = append(indices, index)
	}
	slices.Sort(indices)
	for _, index := range indices {
		result.ToolCalls = append(result.ToolCalls, *s.tool[index])
	}
	return result
}
func (s *stream) Err() error { return s.inner.Err() }
func (s *stream) Close() error {
	err := s.inner.Close()
	s.once.Do(func() {
		if s.record != nil {
			recordErr := s.inner.Err()
			if recordErr == nil {
				recordErr = err
			}
			s.record(context.WithoutCancel(s.ctx), s.config, s.purpose, s.Result(), time.Since(s.start), recordErr)
		}
	})
	return err
}
