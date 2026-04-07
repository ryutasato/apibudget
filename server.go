package apibudget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Server はHTTP APIサーバー。BudgetManagerをREST APIとして公開する。
type Server struct {
	manager      *BudgetManager
	addr         string
	reservations map[string]*Reservation
	mu           sync.Mutex
}

// NewServer はAPIサーバーを生成する。
func NewServer(manager *BudgetManager, addr string) *Server {
	return &Server{
		manager:      manager,
		addr:         addr,
		reservations: make(map[string]*Reservation),
	}
}

// Start はサーバーを起動する。コンテキストキャンセルでグレースフルシャットダウンする。
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/allow", s.handleAllow)
	mux.HandleFunc("/api/v1/reserve", s.handleReserve)
	mux.HandleFunc("/api/v1/wait", s.handleWait)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	// Pattern-based routes handled via prefix matching
	mux.HandleFunc("/api/v1/reserve/", s.handleReserveActions)
	mux.HandleFunc("/api/v1/credits/", s.handleCredits)
	mux.HandleFunc("/api/v1/tokens/", s.handleTokens)
}

// Handler returns the HTTP handler for testing purposes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// --- Request/Response types ---

type allowRequest struct {
	API string `json:"api"`
	N   int64  `json:"n"`
}

type allowResponse struct {
	Allowed       bool   `json:"allowed"`
	NextAvailable string `json:"next_available"`
}

type reserveRequest struct {
	API string `json:"api"`
	N   int64  `json:"n"`
}

type reserveResponse struct {
	ReservationID string `json:"reservation_id"`
	OK            bool   `json:"ok"`
	DelayMs       int64  `json:"delay_ms"`
}

type confirmRequest struct {
	ActualCost string `json:"actual_cost"`
}

type confirmResponse struct {
	Confirmed bool   `json:"confirmed"`
	Error     string `json:"error,omitempty"`
}

type cancelResponse struct {
	Canceled bool `json:"cancelled"`
}

type waitRequest struct {
	API       string `json:"api"`
	N         int64  `json:"n"`
	TimeoutMs int64  `json:"timeout_ms"`
}

type waitResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type creditsResponse struct {
	Pool      string `json:"pool"`
	Remaining string `json:"remaining"`
	Max       string `json:"max,omitempty"`
}

type tokensResponse struct {
	API    string  `json:"api"`
	Tokens float64 `json:"tokens"`
}

type healthResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// --- Handlers ---

func (s *Server) handleAllow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req allowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.API == "" {
		writeError(w, http.StatusBadRequest, "api is required")
		return
	}
	if req.N <= 0 {
		req.N = 1
	}

	allowed, nextAvail := s.manager.AllowN(req.API, req.N, time.Now())

	resp := allowResponse{
		Allowed: allowed,
	}
	if !nextAvail.IsZero() {
		resp.NextAvailable = nextAvail.Format(time.RFC3339Nano)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.API == "" {
		writeError(w, http.StatusBadRequest, "api is required")
		return
	}
	if req.N <= 0 {
		req.N = 1
	}

	reservation := s.manager.ReserveN(req.API, req.N, time.Now())
	id := uuid.New().String()

	s.mu.Lock()
	s.reservations[id] = reservation
	s.mu.Unlock()

	resp := reserveResponse{
		ReservationID: id,
		OK:            reservation.OK(),
		DelayMs:       reservation.Delay().Milliseconds(),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReserveActions(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/v1/reserve/{id}/confirm or /api/v1/reserve/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/reserve/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "reservation id required")
		return
	}

	id := parts[0]

	if len(parts) == 2 && parts[1] == "confirm" {
		s.handleConfirm(w, r, id)
		return
	}

	// DELETE /api/v1/reserve/{id}
	if r.Method == http.MethodDelete {
		s.handleCancel(w, r, id)
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.mu.Lock()
	reservation, ok := s.reservations[id]
	s.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, "reservation not found")
		return
	}

	actualCost, err := NewCredit(req.ActualCost)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid actual_cost: %v", err))
		return
	}

	confirmErr := reservation.Confirm(actualCost)

	resp := confirmResponse{
		Confirmed: true,
	}
	if confirmErr != nil {
		resp.Error = confirmErr.Error()
	}

	// Clean up reservation after confirm
	s.mu.Lock()
	delete(s.reservations, id)
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	reservation, ok := s.reservations[id]
	if ok {
		delete(s.reservations, id)
	}
	s.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, "reservation not found")
		return
	}

	reservation.Cancel()

	writeJSON(w, http.StatusOK, cancelResponse{Canceled: true})
}

func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req waitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.API == "" {
		writeError(w, http.StatusBadRequest, "api is required")
		return
	}
	if req.N <= 0 {
		req.N = 1
	}

	ctx := r.Context()
	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	err := s.manager.WaitN(ctx, req.API, req.N)
	if err != nil {
		writeJSON(w, http.StatusOK, waitResponse{Success: false, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, waitResponse{Success: true})
}

func (s *Server) handleCredits(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/v1/credits/{pool} or /api/v1/credits/{pool}/reset
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/credits/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "pool name required")
		return
	}

	poolName := parts[0]

	if len(parts) == 2 && parts[1] == "reset" {
		s.handleCreditsReset(w, r, poolName)
		return
	}

	// GET /api/v1/credits/{pool}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	remaining, err := s.manager.GetCredits(poolName)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	pool, ok := s.manager.pools[poolName]
	if !ok {
		writeError(w, http.StatusNotFound, "pool not found")
		return
	}

	writeJSON(w, http.StatusOK, creditsResponse{
		Pool:      poolName,
		Remaining: remaining.String(),
		Max:       pool.MaxCredits.String(),
	})
}

func (s *Server) handleCreditsReset(w http.ResponseWriter, r *http.Request, poolName string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := s.manager.ResetCredits(poolName); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	remaining, _ := s.manager.GetCredits(poolName)

	writeJSON(w, http.StatusOK, creditsResponse{
		Pool:      poolName,
		Remaining: remaining.String(),
	})
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	apiName := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")
	if apiName == "" {
		writeError(w, http.StatusNotFound, "api name required")
		return
	}

	tokens := s.manager.Tokens(apiName)

	writeJSON(w, http.StatusOK, tokensResponse{
		API:    apiName,
		Tokens: tokens,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
