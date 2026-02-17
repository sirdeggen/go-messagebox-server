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
	sdk "github.com/bsv-blockchain/go-sdk/wallet"
	"github.com/bsv-blockchain/go-wallet-toolbox/pkg/defs"
	"github.com/bsv-blockchain/go-wallet-toolbox/pkg/services"
	"github.com/bsv-blockchain/go-wallet-toolbox/pkg/storage"
	toolboxwallet "github.com/bsv-blockchain/go-wallet-toolbox/pkg/wallet"
	"github.com/bsv-blockchain/go-wallet-toolbox/pkg/wdk"
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

	// Create production wallet using go-wallet-toolbox
	w, walletCleanup, err := createWallet(cfg)
	if err != nil {
		slog.Error("failed to create wallet", "error", err)
		os.Exit(1)
	}
	defer walletCleanup()

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

func createWallet(cfg *config.Config) (sdk.Interface, func(), error) {
	network := defs.NetworkMainnet
	if cfg.BSVNetwork == "testnet" {
		network = defs.NetworkTestnet
	}

	svcConfig := defs.DefaultServicesConfig(network)
	walletServices := services.New(slog.Default(), svcConfig)

	// TODO: support remote storage via cfg.WalletStorageURL using storage.NewClient

	// Use local SQLite storage
	dbConfig := defs.DefaultDBConfig()
	dbConfig.SQLite.ConnectionString = "wallet-storage.sqlite"

	activeStorage, err := storage.NewGORMProvider(network, walletServices,
		storage.WithDBConfig(dbConfig),
		storage.WithLogger(slog.Default()),
		storage.WithBackgroundBroadcasterContext(context.Background()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create local storage: %w", err)
	}

	storageIdentityKey, err := wdk.IdentityKey(cfg.ServerPrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive storage identity key: %w", err)
	}

	_, err = activeStorage.Migrate(context.Background(), "messagebox-storage", storageIdentityKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to migrate storage: %w", err)
	}

	w, err := toolboxwallet.New(network, cfg.ServerPrivateKey, activeStorage)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	return w, func() {
		activeStorage.Stop()
	}, nil
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
