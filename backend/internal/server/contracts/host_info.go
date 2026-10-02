package contracts

import (
	"context"
	"database/sql"
)

const HostPairingInfoKey = "hosts.pairing_info"

type HostInfoInput struct {
	Ownership     *string   `json:"ownership,omitempty"`
	Client        *string   `json:"client,omitempty"`
	Username      *string   `json:"username,omitempty"`
	Password      *string   `json:"password,omitempty"`
	ClearPassword *bool     `json:"clearPassword,omitempty"`
	Note          *string   `json:"note,omitempty"`
	Tags          *[]string `json:"tags,omitempty"`
}

type HostPairingInfo interface {
	Validate(context.Context, HostInfoInput) error
	SavePairing(context.Context, *sql.Tx, string, HostInfoInput) error
	ApplyPairing(context.Context, *sql.Tx, string, string, string) error
}
