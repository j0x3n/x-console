package contracts

import "context"

// TelegramInboxKey is where a module that takes plain Telegram messages
// registers itself (B117: the read-later module).
const TelegramInboxKey = "telegram.inbox"

// TelegramMessage is a plain message the user sent to the Telegram bot.
type TelegramMessage struct {
	Text string
	// Links are the addresses hidden behind link text. Addresses written out in
	// Text are not repeated here.
	Links []string
}

// TelegramInbox receives plain messages from the configured chat. The
// reminders module checks the webhook secret and the chat first. When handled
// is true, reply is sent back to the chat (unless empty).
type TelegramInbox interface {
	HandleTelegramMessage(ctx context.Context, msg TelegramMessage) (reply string, handled bool)
}
