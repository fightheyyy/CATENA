package control

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const apiTokenEnvelopeVersion = "v1"

func encryptAPIToken(plaintext string, tokenID string, secret string) (string, error) {
	if plaintext == "" || tokenID == "" || len(secret) < 32 {
		return "", errors.New("API token encryption is not configured")
	}
	block, err := aes.NewCipher(apiTokenCipherKey(secret))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(plaintext), []byte(tokenID))
	payload := append(nonce, sealed...)
	return apiTokenEnvelopeVersion + "." + base64.RawURLEncoding.EncodeToString(payload), nil
}

func decryptAPIToken(envelope string, tokenID string, secret string) (string, error) {
	version, encoded, ok := strings.Cut(envelope, ".")
	if !ok || version != apiTokenEnvelopeVersion || tokenID == "" || len(secret) < 32 {
		return "", errors.New("invalid API token envelope")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("invalid API token envelope")
	}
	// Raw URL base64 can decode multiple non-canonical final characters to the
	// same bytes when the payload does not end on a six-bit boundary. Require
	// the canonical spelling so any textual mutation is rejected as tampering.
	if base64.RawURLEncoding.EncodeToString(payload) != encoded {
		return "", errors.New("invalid API token envelope")
	}
	block, err := aes.NewCipher(apiTokenCipherKey(secret))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) <= aead.NonceSize() {
		return "", errors.New("invalid API token envelope")
	}
	nonce, ciphertext := payload[:aead.NonceSize()], payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(tokenID))
	if err != nil {
		return "", errors.New("API token recovery failed")
	}
	return string(plaintext), nil
}

func apiTokenCipherKey(secret string) []byte {
	sum := sha256.Sum256([]byte("catena:api-token-recovery:v1:" + secret))
	return sum[:]
}

func maskAPIToken(plaintext string) string {
	if !strings.HasPrefix(plaintext, "barena_pat_") || len(plaintext) < 4 {
		return "barena_pat_••••••••"
	}
	return fmt.Sprintf("barena_pat_••••••••%s", plaintext[len(plaintext)-4:])
}
