package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mlavacca/firectl/internal/config"
	"github.com/mlavacca/firectl/internal/firefly"
	"github.com/mlavacca/firectl/internal/processor"
)

var (
	serverPort string
	serverAddr string
)

// importResultJSON is a JSON-serializable version of processor.Result.
type importResultJSON struct {
	Row    int    `json:"row"`
	Status string `json:"status"`
	ID     string `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

var serverCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the firectl HTTP server",
	Long: `Starts an HTTP server that exposes firectl commands as HTTP endpoints.

Endpoints:
  GET  /rules  - List all Firefly III rules (sanitized, instance-specific fields removed)
  POST /import - Import transactions from a CSV file

  The /import endpoint expects a multipart/form-data request with:
    file     (required) - the CSV file to import
    provider (required) - bank provider (satispay, sanpaolo)
    dry_run  (optional) - set to "true" or "1" to skip creating transactions

Configuration:
  - FIREFLY_URL and FIREFLY_TOKEN environment variables (or .env file)

Example:
  firectl serve
  firectl serve --port 9090 --addr 127.0.0.1`,
	Args: cobra.NoArgs,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&serverPort, "port", "p", "8080", "Port to listen on")
	serverCmd.Flags().StringVarP(&serverAddr, "addr", "a", "0.0.0.0", "Address to bind to")
}

func runServer(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rules", makeRulesHandler(cfg))
	mux.HandleFunc("POST /import", makeImportHandler(cfg))

	addr := net.JoinHostPort(serverAddr, serverPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		fmt.Fprintf(os.Stderr, "Shutting down server...\n")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	fmt.Fprintf(os.Stderr, "Server listening on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func makeRulesHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		client := firefly.NewClient(cfg.FireflyURL, cfg.FireflyToken)

		rules, err := client.ListRules()
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sanitizeRules(rules))
	}
}

func makeImportHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}

		providerName := r.FormValue("provider")
		if providerName == "" {
			writeJSONError(w, http.StatusBadRequest, "provider field is required")
			return
		}

		dryRunVal := r.FormValue("dry_run")
		dryRun := dryRunVal == "true" || dryRunVal == "1"

		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "file field is required")
			return
		}
		defer func() { _ = file.Close() }()

		tmpPath, err := saveTempCSV(file)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save uploaded file")
			return
		}
		defer func() { _ = os.Remove(tmpPath) }()

		proc := processor.NewProcessor(cfg)

		results, err := proc.Process(tmpPath, providerName, dryRun)
		if err != nil {
			writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		resp := make([]importResultJSON, 0, len(results))
		for _, res := range results {
			item := importResultJSON{
				Row:    res.Row,
				Status: res.Status,
				ID:     res.ID,
			}
			if res.Error != nil {
				item.Error = res.Error.Error()
			}
			resp = append(resp, item)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func saveTempCSV(src multipart.File) (string, error) {
	tmp, err := os.CreateTemp("", "firectl-import-*.csv")
	if err != nil {
		return "", err
	}
	defer func() { _ = tmp.Close() }()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}
