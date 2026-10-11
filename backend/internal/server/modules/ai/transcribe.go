package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// DefaultTranscribeModel is used when the caller names no model.
const DefaultTranscribeModel = "whisper-1"

// maxTranscribeBytes is the largest audio the module sends to a provider.
const maxTranscribeBytes = 25 << 20

// Transcribe implements contracts.Transcriber (B134). It sends the audio to
// the OpenAI compatible /audio/transcriptions endpoint of the provider chosen
// for fast AI calls. Only the audio and the language hint leave the server.
func (m *Module) Transcribe(ctx context.Context, in contracts.Transcribe) (string, error) {
	if len(in.Audio) == 0 {
		return "", errors.New("没有声音内容")
	}
	if len(in.Audio) > maxTranscribeBytes {
		return "", errors.New("录音太大了，最多 25 MB")
	}
	cfg, err := m.resolveLLM(ctx, "fast")
	if err != nil {
		if errors.Is(err, llm.ErrNotConfigured) {
			return "", errors.New("还没有配置 AI 供应商，不能转写")
		}
		return "", err
	}
	key := cfg.APIKey
	if key == "" {
		key = "local-model"
	}
	client := openai.NewClient(option.WithBaseURL(cfg.BaseURL), option.WithAPIKey(key))
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = DefaultTranscribeModel
	}
	name := in.Filename
	if name == "" {
		name = "voice.ogg"
	}
	contentType := in.Mime
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	params := openai.AudioTranscriptionNewParams{
		File:  openai.File(bytes.NewReader(in.Audio), name, contentType),
		Model: openai.AudioModel(model),
	}
	if in.Language != "" {
		params.Language = openai.String(in.Language)
	}
	res, err := client.Audio.Transcriptions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("转写失败：%w", err)
	}
	return strings.TrimSpace(res.Text), nil
}
