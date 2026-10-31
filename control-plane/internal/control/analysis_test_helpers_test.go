package control

import (
	"context"
	"strings"
	"testing"
	"time"
)

const testAnalysisEncryptionKey = "catena-test-evolution-encryption-key-0001"

func testAnalysisAuthConfig() AuthConfig {
	return AuthConfig{APITokenEncryptionKey: testAnalysisEncryptionKey}
}

func seedTestAnalysisModelConfig(t *testing.T, store Store, ownerUserID string) {
	t.Helper()
	if strings.TrimSpace(ownerUserID) == "" {
		ownerUserID = "local"
	}
	encrypted, err := encryptAPIToken(
		"test-model-api-key",
		analysisModelEnvelopePrefix+ownerUserID,
		testAnalysisEncryptionKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertAnalysisModelConfig(context.Background(), AnalysisModelConfig{
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

func mustHTTPHandler(store Store) *ManagedHTTPHandler {
	h, e := NewHTTPHandler(store, HTTPConfig{})
	if e != nil {
		panic(e)
	}
	return h
}
