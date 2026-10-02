package repo

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

type config struct {
	Format      int    `json:"format"`
	ID          string `json:"id"`
	Salt        []byte `json:"salt"`
	Owner       string `json:"owner"`
	ChunkSize   int    `json:"chunkSize"`
	KDF         string `json:"kdf"`
	Cipher      string `json:"cipher"`
	Compression string `json:"compression"`
	Auth        []byte `json:"auth,omitempty"`
}

type envelope struct {
	Format int    `json:"format"`
	Data   []byte `json:"data"`
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func derive(master, salt []byte, purpose string) ([]byte, error) {
	key := make([]byte, 32)
	_, err := io.ReadFull(hkdf.New(sha256.New, master, salt, []byte("x-console-repo/v1/"+purpose)), key)
	return key, err
}

func configAuth(c config, key []byte) []byte {
	c.Auth = nil
	raw, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	return mac.Sum(nil)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(a cipher.AEAD, plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, aad), nil
}

func unseal(a cipher.AEAD, raw, aad []byte) ([]byte, error) {
	if len(raw) < a.NonceSize()+a.Overhead() {
		return nil, ErrCorrupt
	}
	plain, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], aad)
	if err != nil {
		return nil, fmt.Errorf("%w：认证校验失败", ErrCorrupt)
	}
	return plain, nil
}

func (r *Repository) aad(kind, id string, size int64) []byte {
	return []byte(fmt.Sprintf("%s/v1/%s/%s/%d", r.cfg.ID, kind, id, size))
}

func (r *Repository) encodeBlock(plain []byte, hash string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(plain); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return seal(r.blobKey, buf.Bytes(), r.aad("blob", hash, int64(len(plain))))
}

func (r *Repository) decodeBlock(raw []byte, b Block) ([]byte, error) {
	if int64(len(raw)) != b.StoredSize {
		return nil, fmt.Errorf("%w：块大小不一致", ErrCorrupt)
	}
	compressed, err := unseal(r.blobKey, raw, r.aad("blob", b.Hash, b.Size))
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("%w：压缩内容不正确", ErrCorrupt)
	}
	defer gz.Close()
	gz.Multistream(false)
	plain, err := io.ReadAll(io.LimitReader(gz, b.Size+1))
	if err != nil || int64(len(plain)) != b.Size {
		return nil, fmt.Errorf("%w：块解压大小不一致", ErrCorrupt)
	}
	sum := sha256.Sum256(plain)
	if hex.EncodeToString(sum[:]) != b.Hash {
		return nil, fmt.Errorf("%w：块内容校验失败", ErrCorrupt)
	}
	return plain, nil
}
