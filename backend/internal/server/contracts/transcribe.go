package contracts

import "context"

// TranscriberKey is where the ai module registers Transcriber (B134).
const TranscriberKey = "ai.transcriber"

// Transcribe is one speech-to-text request.
type Transcribe struct {
	Audio    []byte
	Filename string // with an extension that tells the format, such as voice.ogg
	Mime     string
	// Model overrides the default transcription model.
	Model string
	// Language is an ISO-639-1 code. Empty lets the model detect it.
	Language string
}

// Transcriber turns speech into text with the AI provider the user configured
// (an OpenAI compatible /audio/transcriptions endpoint).
type Transcriber interface {
	// Available says whether a provider is configured.
	Available(ctx context.Context) bool
	Transcribe(ctx context.Context, in Transcribe) (string, error)
}
