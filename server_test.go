package apibudget

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestServer creates a Server with a simple BudgetManager for testing.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	manager, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{
				Name: "test_api",
				Windows: []Window{
					{Duration: time.Minute, Limit: 10},
				},
			},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "test_pool",
				MaxCredits: MustNewCredit("100"),
				Costs: []CreditCost{
					{APIName: "test_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
		LogLevel: LogLevelSilent,
	})
	if err != nil {
		t.Fatalf("failed to create BudgetManager: %v", err)
	}
	return NewServer(manager, ":0")
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %q", resp.Status)
	}
}

func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/health", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestServerDefaultTimeouts(t *testing.T) {
	s := newTestServer(t)
	if s.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("expected ReadHeaderTimeout 10s, got %v", s.ReadHeaderTimeout)
	}
	if s.ReadTimeout != 30*time.Second {
		t.Errorf("expected ReadTimeout 30s, got %v", s.ReadTimeout)
	}
	if s.WriteTimeout != 60*time.Second {
		t.Errorf("expected WriteTimeout 60s, got %v", s.WriteTimeout)
	}
	if s.IdleTimeout != 120*time.Second {
		t.Errorf("expected IdleTimeout 120s, got %v", s.IdleTimeout)
	}
}

func TestAllowEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{API: "test_api", N: 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp allowResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Allowed {
		t.Error("expected allowed to be true")
	}
}

func TestAllowEndpoint_MissingAPI(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{N: 1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDecodeJSONBody_InvalidJSON(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	body := bytes.NewBufferString(`{"api": "test_api", "n": `) // Malformed JSON
	req := httptest.NewRequest(http.MethodPost, "/api/v1/allow", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "invalid request body" {
		t.Errorf("expected error 'invalid request body', got %q", resp.Error)
	}
}

func TestAllowEndpoint_UnknownAPI(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{API: "unknown", N: 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp allowResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Allowed {
		t.Error("expected allowed to be false for unknown API")
	}
}

func TestAllowEndpoint_RateLimited(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	// Exhaust the rate limit (10 requests)
	for i := 0; i < 10; i++ {
		rec := doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{API: "test_api", N: 1})
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	// 11th request should be rate limited
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{API: "test_api", N: 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp allowResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Allowed {
		t.Error("expected allowed to be false after rate limit exhausted")
	}
	if resp.NextAvailable == "" {
		t.Error("expected next_available to be set")
	}
}

func TestReserveAndConfirmEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	// Reserve
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/reserve", reserveRequest{API: "test_api", N: 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resResp reserveResponse
	if err := json.NewDecoder(rec.Body).Decode(&resResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resResp.OK {
		t.Error("expected reservation ok to be true")
	}
	if resResp.ReservationID == "" {
		t.Error("expected reservation_id to be set")
	}

	// Confirm
	rec = doRequest(t, handler, http.MethodPost, "/api/v1/reserve/"+resResp.ReservationID+"/confirm",
		confirmRequest{ActualCost: "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var confResp confirmResponse
	if err := json.NewDecoder(rec.Body).Decode(&confResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !confResp.Confirmed {
		t.Error("expected confirmed to be true")
	}
	if confResp.Error != "" {
		t.Errorf("expected no error, got %q", confResp.Error)
	}
}

func TestReserveAndCancelEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	// Reserve
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/reserve", reserveRequest{API: "test_api", N: 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resResp reserveResponse
	if err := json.NewDecoder(rec.Body).Decode(&resResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Cancel
	rec = doRequest(t, handler, http.MethodDelete, "/api/v1/reserve/"+resResp.ReservationID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var cancelResp cancelResponse
	if err := json.NewDecoder(rec.Body).Decode(&cancelResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !cancelResp.Canceled {
		t.Error("expected canceled to be true")
	}
}

func TestConfirmEndpoint_NotFound(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/reserve/nonexistent/confirm",
		confirmRequest{ActualCost: "1"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCancelEndpoint_NotFound(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodDelete, "/api/v1/reserve/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestWaitEndpoint_Success(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/wait",
		waitRequest{API: "test_api", N: 1, TimeoutMs: 1000})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp waitResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success to be true, got error: %s", resp.Error)
	}
}

func TestWaitEndpoint_MissingAPI(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/wait",
		waitRequest{N: 1, TimeoutMs: 1000})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreditsEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/credits/test_pool", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp creditsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Pool != "test_pool" {
		t.Errorf("expected pool test_pool, got %q", resp.Pool)
	}
	if resp.Remaining != "100" {
		t.Errorf("expected remaining 100, got %q", resp.Remaining)
	}
	if resp.Max != "100" {
		t.Errorf("expected max 100, got %q", resp.Max)
	}
}

func TestCreditsEndpoint_NotFound(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/credits/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreditsResetEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	// First consume some credits
	doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{API: "test_api", N: 1})

	// Reset
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/credits/test_pool/reset", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp creditsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Pool != "test_pool" {
		t.Errorf("expected pool test_pool, got %q", resp.Pool)
	}
	if resp.Remaining != "100" {
		t.Errorf("expected remaining 100 after reset, got %q", resp.Remaining)
	}
}

func TestCreditsResetEndpoint_NotFound(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/credits/nonexistent/reset", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTokensEndpoint(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/tokens/test_api", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp tokensResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.API != "test_api" {
		t.Errorf("expected api test_api, got %q", resp.API)
	}
	if resp.Tokens != 10 {
		t.Errorf("expected tokens 10, got %f", resp.Tokens)
	}
}

func TestTokensEndpoint_UnknownAPI(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/tokens/unknown", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp tokensResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Tokens != 0 {
		t.Errorf("expected tokens 0 for unknown API, got %f", resp.Tokens)
	}
}

func TestAllowEndpoint_DefaultN(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	// N=0 should default to 1
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/allow", allowRequest{API: "test_api"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp allowResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Allowed {
		t.Error("expected allowed to be true with default N")
	}
}

func TestReserveEndpoint_MissingAPI(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodPost, "/api/v1/reserve", reserveRequest{N: 1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAllowEndpoint_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	rec := doRequest(t, handler, http.MethodGet, "/api/v1/allow", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestConfirmEndpoint_InvalidCost(t *testing.T) {
	s := newTestServer(t)
	handler := s.Handler()

	// Reserve first
	rec := doRequest(t, handler, http.MethodPost, "/api/v1/reserve", reserveRequest{API: "test_api", N: 1})
	var resResp reserveResponse
	_ = json.NewDecoder(rec.Body).Decode(&resResp)

	// Confirm with invalid cost
	rec = doRequest(t, handler, http.MethodPost, "/api/v1/reserve/"+resResp.ReservationID+"/confirm",
		confirmRequest{ActualCost: "invalid"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
