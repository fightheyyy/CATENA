package control

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func newID(prefix string) string {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().UnixMilli(), hex.EncodeToString(entropy[:]))
}

func bounded(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
func validateReplayIdempotencyKey(value string) error {
	if value == "" || len(value) > 200 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("Idempotency-Key must contain from 1 to 200 safe characters")
	}
	return nil
}

func resourceOwnedBy(ownerUserID string, user *User) bool {
	return user == nil || ownerUserID == user.ID
}

func analysisListLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 100, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 1000 {
		return 0, errors.New("limit must be from 1 to 1000")
	}
	return value, nil
}

func cloneJSON(v json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), v...) }
