package control

import (
	"context"
	"strings"
	"testing"
	"time"
)

const testEvolutionEncryptionKey = "catena-test-evolution-encryption-key-0001"

func testEvolutionAuthConfig() AuthConfig {
	return AuthConfig{APITokenEncryptionKey: testEvolutionEncryptionKey}
}

func seedTestEvolutionModelConfig(t *testing.T, store *MemoryStore, ownerUserID string) {
	t.Helper()
	if strings.TrimSpace(ownerUserID) == "" {
		ownerUserID = "local"
	}
	encrypted, err := encryptAPIToken(
		"test-model-api-key",
		evolutionModelEnvelopePrefix+ownerUserID,
		testEvolutionEncryptionKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertEvolutionModelConfig(context.Background(), EvolutionModelConfig{
		OwnerUserID:     ownerUserID,
		Provider:        "openai",
		BaseURL:         "https://llm.example.test/v1",
		Model:           "test-model",
		EncryptedAPIKey: encrypted,
		UpdatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
}
