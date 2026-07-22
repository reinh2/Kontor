package tenants

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

const channelKeyBytes = 32

type secretCipher struct{ aead cipher.AEAD }

func newSecretCipher(key []byte) (*secretCipher, error) {
	if len(key) != channelKeyBytes {
		return nil, fmt.Errorf("tenant channels: encryption key must contain %d bytes", channelKeyBytes)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("tenant channels: construct cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("tenant channels: construct GCM: %w", err)
	}
	return &secretCipher{aead: aead}, nil
}

func (c *secretCipher) seal(value string) (ciphertext, nonce []byte, err error) {
	if c == nil || c.aead == nil {
		return nil, nil, errors.New("tenant channels: encryption is not configured")
	}
	nonce = make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("tenant channels: generate nonce: %w", err)
	}
	return c.aead.Seal(nil, nonce, []byte(value), nil), nonce, nil
}

func (c *secretCipher) open(ciphertext, nonce []byte) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("tenant channels: encryption is not configured")
	}
	if len(nonce) != c.aead.NonceSize() || len(ciphertext) == 0 {
		return "", errors.New("tenant channels: stored ciphertext is malformed")
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("tenant channels: decrypt stored secret")
	}
	return string(plaintext), nil
}

func webhookDigest(secret string) [sha256.Size]byte { return sha256.Sum256([]byte(secret)) }
