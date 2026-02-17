package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bsv-blockchain/go-messagebox-server/internal/config"
	"github.com/bsv-blockchain/go-messagebox-server/internal/db"
	"github.com/bsv-blockchain/go-messagebox-server/internal/handlers"
	"github.com/bsv-blockchain/go-messagebox-server/internal/logger"
	"github.com/bsv-blockchain/go-bsv-middleware/pkg/middleware"
	"github.com/bsv-blockchain/go-sdk/wallet"
	"github.com/go-softwarelab/common/pkg/testingx"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if cfg.NodeEnv == "development" {
		logger.Enable()
	}

	// Open DB and run migrations
	database, err := db.New(cfg.DBDriver, cfg.DBSource)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Create wallet from private key hex
	w := wallet.NewTestWallet(&testingx.E{Verbose: true}, wallet.PrivHex(cfg.ServerPrivateKey))

	srv := &handlers.Server{DB: database}

	// Build router
	mux := http.NewServeMux()

	prefix := cfg.RoutingPrefix

	// All routes require auth (postAuth in the original)
	mux.HandleFunc("POST "+prefix+"/sendMessage", srv.SendMessage)
	mux.HandleFunc("POST "+prefix+"/listMessages", srv.ListMessages)
	mux.HandleFunc("POST "+prefix+"/acknowledgeMessage", srv.AcknowledgeMessage)
	mux.HandleFunc("POST "+prefix+"/registerDevice", srv.RegisterDevice)
	mux.HandleFunc("GET "+prefix+"/devices", srv.ListDevices)
	mux.HandleFunc("POST "+prefix+"/permissions/set", srv.SetPermission)
	mux.HandleFunc("GET "+prefix+"/permissions/get", srv.GetPermission)
	mux.HandleFunc("GET "+prefix+"/permissions/list", srv.ListPermissions)
	mux.HandleFunc("GET "+prefix+"/permissions/quote", srv.GetQuote)

	// Auth middleware
	authMiddleware := middleware.NewAuth(w)

	// Payment middleware (returns 0 for now, matching the original)
	paymentMiddleware := middleware.NewPayment(w, middleware.WithRequestPriceCalculator(func(r *http.Request) (int, error) {
		return 0, nil
	}))

	// Stack: CORS -> Auth -> Payment -> Routes
	handler := &corsHandler{
		next: authMiddleware.HTTPHandler(
			paymentMiddleware.HTTPHandler(mux),
		),
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Log("MessageBox listening", "port", cfg.Port)
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Log("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

type corsHandler struct {
	next http.Handler
}

func (h *corsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Expose-Headers", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	h.next.ServeHTTP(w, r)
}
