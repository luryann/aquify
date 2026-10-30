package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"
)

func signedHeader(payload []byte, secret string, ts time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", ts.Unix(), payload)
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyStripeSignatureRejectsFarFutureTimestamp(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	const secret = "whsec_test"
	if err := VerifyStripeSignature(payload, signedHeader(payload, secret, time.Now()), secret, 0); err != nil {
		t.Fatalf("current timestamp rejected: %v", err)
	}
	err := VerifyStripeSignature(payload, signedHeader(payload, secret, time.Now().Add(24*time.Hour)), secret, 0)
	if !errors.Is(err, ErrExpiredTimestamp) {
		t.Fatalf("future-dated signature: want ErrExpiredTimestamp, got %v", err)
	}
}
