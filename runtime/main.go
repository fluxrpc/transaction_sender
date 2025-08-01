package main

import (
	"context"
	"github.com/alphabatem/solana_transaction_sender"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	r := runtime{}
	if err := r.run(); err != nil {
		log.Fatal(err)
	}
}

type runtime struct {
	sts *solana_transaction_sender.TransactionSender
}

func (rt *runtime) run() error {
	var err error
	rt.sts, err = solana_transaction_sender.NewTransactionSender(os.Getenv("RPC_URL"))
	if err != nil {
		return err
	}

	err = rt.sts.Load(context.TODO())
	if err != nil {
		return err
	}

	http.HandleFunc("/", rt.sendTransaction)
	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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
