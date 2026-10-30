package usaswimming

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func swimsServer(t *testing.T, tokens *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_secret") != "sekret" {
			w.WriteHeader(401)
			return
		}
		n := atomic.AddInt32(tokens, 1)
		_, _ = w.Write([]byte(`{"access_token":"tok` + string(rune('0'+n)) + `","expires_in":3600}`))
	})
	mux.HandleFunc("/swims/SwimsThirdParty/VendorClubs/vendor", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`[{"clubId":"c1","clubName":"DARE","clubCode":"DARE"}]`))
	})
	mux.HandleFunc("/swims/SwimsThirdParty/MemberDetails/club/c1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		_, _ = w.Write([]byte(`[{"memberId":"ABC123","firstName":"Ann","lastName":"Lee","birthDate":"2012-04-05T00:00:00","competitionCategory":"female"}]`))
	})
	return httptest.NewServer(mux)
}

func TestSwimsClient(t *testing.T) {
	var tokens int32
	srv := swimsServer(t, &tokens)
	defer srv.Close()
	cfg := SwimsConfig{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "id", ClientSecret: "sekret", AllowLoopback: true}
	c, err := NewSwimsClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("test connection: %v", err)
	}
	clubs, err := c.Clubs(ctx)
	if err != nil || len(clubs) != 1 || clubs[0].ClubID != "c1" {
		t.Fatalf("clubs %+v %v", clubs, err)
	}
	if tokens != 1 {
		t.Fatalf("token should be cached, fetched %d times", tokens)
	}
	roster, err := c.Roster(ctx, "c1", nil)
	if err != nil || len(roster) != 1 || roster[0].DateOfBirth != "2012-04-05" || roster[0].CompetitionCategory != "FEMALE" {
		t.Fatalf("roster %+v %v", roster, err)
	}
	if _, err := c.OfficialTimes(ctx, "ABC123"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("official times: %v", err)
	}

	cfg.ClientSecret = "wrong"
	bad, _ := NewSwimsClient(cfg)
	if err := bad.TestConnection(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad secret: %v", err)
	}
	if _, err := NewSwimsClient(SwimsConfig{BaseURL: "http://x.example.com", TokenURL: "https://t.example.com", ClientID: "i", ClientSecret: "s"}); err == nil {
		t.Fatal("http base URL should be rejected")
	}
}

func TestValidateRegistration(t *testing.T) {
	ok := Registration{FirstName: "Ann", LastName: "O'Lee", DateOfBirth: "2012-04-05", CompetitionCategory: "female", Phone: "15551234567", PhoneType: "Mobile", PostalCode: "93106-1234"}
	if p := ValidateRegistration(ok); len(p) != 0 {
		t.Fatalf("valid payload rejected: %v", p)
	}
	bad := Registration{MemberID: "short", FirstName: "A1", LastName: "", DateOfBirth: "04/05/2012", CompetitionCategory: "UNKNOWN", Phone: "555", PhoneType: "Fax", PostalCode: "9310"}
	if p := ValidateRegistration(bad); len(p) != 7 {
		t.Fatalf("want 7 problems, got %d: %v", len(p), p)
	}
}
