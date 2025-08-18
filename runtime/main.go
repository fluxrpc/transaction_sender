package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/fluxrpc/transaction_sender"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	r := runtime{}
	if err := r.run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start runtime")
	}
}

type runtime struct {
	sts *transaction_sender.TransactionSender
}

func (rt *runtime) run() error {
	httpPortFlag := flag.String("http_port", "", "Port for HTTP server")
	rpcURLFlag := flag.String("rpc_url", "", "RPC URL for Solana")
	flag.Parse()

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" && *httpPortFlag != "" {
		httpPort = *httpPortFlag
	}
	if httpPort == "" {
		httpPort = "8080"
	}

	rpcURL := os.Getenv("RPC_URL")
	if rpcURL == "" && *rpcURLFlag != "" {
		rpcURL = *rpcURLFlag
	}

	var err error
	rt.sts, err = transaction_sender.NewTransactionSender(os.Getenv("RPC_URL"))
	if err != nil {
		return err
	}

	http.HandleFunc("/", rt.sendTransaction)
	log.Info().Msgf("Listening on :%s", httpPort)

	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf(":%s", httpPort), nil)).Msg("Failed to start HTTPServer")
	return nil
}

func (rt *runtime) sendTransaction(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	// Handle preflight request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}
	txBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = rt.sts.Send(ctx, txBytes)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}
