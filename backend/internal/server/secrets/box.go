// Package secrets encrypts values at rest with AES-256-GCM.
// Third-party tokens, SSH keys and the TOTP secret all go through Box.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

// Box seals and opens strings with the server master key.
type Box struct{ aead cipher.AEAD }

// NewBox builds a Box from a 32 byte key.
func NewBox(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext and returns base64(nonce|ciphertext).
func (b *Box) Seal(plaintext string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Open reverses Seal.
func (b *Box) Open(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	n := b.aead.NonceSize()
	if len(raw) < n {
		return "", errors.New("secrets: ciphertext too short")
	}
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
