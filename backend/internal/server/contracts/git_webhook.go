package contracts

import (
	"context"
	"encoding/json"
)

const GitWebhookKey = "github.webhook"

type GitWebhook struct {
	ConnectionID int64
	Event        string
	DeliveryID   string
	Body         json.RawMessage
}

type GitWebhookReceiver interface {
	ReceiveGitWebhook(ctx context.Context, hook GitWebhook) error
}
