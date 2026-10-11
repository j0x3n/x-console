package contracts

import "context"

// TelegramInboxKey is where a module that takes plain Telegram messages
// registers itself (B117: the read-later module).
const TelegramInboxKey = "telegram.inbox"

// TelegramInboxFallbackKey is where the module that takes everything the first
// inbox did not (B134: the capture inbox) registers itself. It is asked only
// when the module at TelegramInboxKey did not handle the message.
const TelegramInboxFallbackKey = "telegram.inbox.fallback"

// TelegramVoice describes a voice message or an audio file.
type TelegramVoice struct {
	Seconds int
	Mime    string
	Size    int64
}

// TelegramMessage is a plain message the user sent to the Telegram bot.
type TelegramMessage struct {
	Text string
	// Links are the addresses hidden behind link text. Addresses written out in
	// Text are not repeated here.
	Links []string
	// Voice is set for a voice message or an audio file. Download fetches its
	// content from Telegram (at most 20 MB).
	Voice    *TelegramVoice
	Download func(ctx context.Context) ([]byte, error)
}

// TelegramInbox receives plain messages from the configured chat. The
// reminders module checks the webhook secret and the chat first. When handled
// is true, reply is sent back to the chat (unless empty).
type TelegramInbox interface {
	HandleTelegramMessage(ctx context.Context, msg TelegramMessage) (reply string, handled bool)
}
