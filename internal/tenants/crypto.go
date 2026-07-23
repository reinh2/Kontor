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

// seal encrypts value under the AEAD, binding the ciphertext to aad. Passing
// the owning tenant's AAD means a stored secret copied into another tenant's
// row fails authentication on open.
func (c *secretCipher) seal(value string, aad []byte) (ciphertext, nonce []byte, err error) {
	if c == nil || c.aead == nil {
		return nil, nil, errors.New("tenant channels: encryption is not configured")
	}
	nonce = make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("tenant channels: generate nonce: %w", err)
	}
	return c.aead.Seal(nil, nonce, []byte(value), aad), nonce, nil
}

// open decrypts and authenticates a stored secret. aad must equal the value
// supplied to seal (the owning tenant's AAD); otherwise authentication fails.
func (c *secretCipher) open(ciphertext, nonce, aad []byte) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("tenant channels: encryption is not configured")
	}
	if len(nonce) != c.aead.NonceSize() || len(ciphertext) == 0 {
		return "", errors.New("tenant channels: stored ciphertext is malformed")
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", errors.New("tenant channels: decrypt stored secret")
	}
	return string(plaintext), nil
}

// channelAAD binds a sealed tenant channel secret to its tenant. A ciphertext
// sealed for one tenant cannot be opened under another tenant's AAD.
func channelAAD(tenantID string) []byte {
	return []byte("kontor.tenant-channel.v1:" + tenantID)
}

func webhookDigest(secret string) [sha256.Size]byte { return sha256.Sum256([]byte(secret)) }
