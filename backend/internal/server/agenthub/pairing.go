package agenthub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func (h *Hub) CreatePairingCodeWithInfo(ctx context.Context, name, kind string, info *contracts.HostInfoInput) (string, time.Time, error) {
	if kind != "server" && kind != "desktop" {
		return "", time.Time{}, httpx.Invalid("kind 必须是 server 或 desktop")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return "", time.Time{}, httpx.Invalid("名称不能为空或过长")
	}
	h.mu.RLock()
	provider := h.pairingInfo
	h.mu.RUnlock()
	if info != nil {
		if provider == nil {
			return "", time.Time{}, httpx.ErrNotLive
		}
		if err := provider.Validate(ctx, *info); err != nil {
			return "", time.Time{}, err
		}
	}
	code := randomCode()
	expires := h.now().Add(pairingTTL)
	hash := secrets.Hash(normalizeCode(code))
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()
	if err = db.New(tx).CreatePairingCode(ctx, db.CreatePairingCodeParams{CodeHash: hash, Name: name, Kind: kind, ExpiresAt: expires}); err != nil {
		return "", time.Time{}, err
	}
	if info != nil {
		if err = provider.SavePairing(ctx, tx, hash, *info); err != nil {
			return "", time.Time{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	h.audit.Record(ctx, "agent.pairing_code", name, map[string]any{"kind": kind}, nil)
	return code, expires, nil
}

func (h *Hub) pairTransaction(ctx context.Context, code string, hello protocol.Hello) (string, string, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	q := db.New(tx)
	hash := secrets.Hash(normalizeCode(code))
	pc, err := q.UsePairingCode(ctx, db.UsePairingCodeParams{UsedAt: ptr(h.now()), CodeHash: hash, ExpiresAt: h.now()})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", httpx.NewError(http.StatusUnauthorized, "invalid_pairing_code", "配对码无效或已过期")
	}
	if err != nil {
		return "", "", err
	}
	id := secrets.RandomID()
	token := secrets.RandomToken(32)
	caps, _ := json.Marshal(nonNil(hello.Capabilities))
	if err = q.CreateAgent(ctx, db.CreateAgentParams{ID: id, Name: pc.Name, Kind: pc.Kind, Os: hello.OS, Arch: hello.Arch, Hostname: hello.Hostname, Version: hello.AgentVersion, Capabilities: string(caps), TokenHash: secrets.Hash(token), CreatedAt: h.now()}); err != nil {
		return "", "", err
	}
	h.mu.RLock()
	provider := h.pairingInfo
	h.mu.RUnlock()
	if provider != nil {
		if err = provider.ApplyPairing(ctx, tx, hash, id, pc.Kind); err != nil {
			return "", "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	h.audit.Record(audit.WithActor(ctx, "agent:"+id), "agent.pair", pc.Name, map[string]any{"hostname": hello.Hostname, "os": hello.OS}, nil)
	h.bus.Publish("agent.paired", map[string]string{"agentId": id})
	return id, token, nil
}

func (h *Hub) SetPairingInfo(provider contracts.HostPairingInfo) {
	h.mu.Lock()
	h.pairingInfo = provider
	h.mu.Unlock()
}
