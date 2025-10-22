//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"syscall/js"
	"time"
)

var sender *TransactionSender

func load(this js.Value, args []js.Value) interface{} {
	rpcEndpoint := args[0].String()
	if rpcEndpoint == "" {
		fmt.Println("RPC endpoint not set")
		return nil
	}

	go func() {
		var err error
		sender, err = NewTransactionSender(rpcEndpoint)
		if err != nil {
			fmt.Println("NewTransactionSender error: ", err)
			return
		}

		fmt.Println("Sender created, loading schedule")
		err = sender.Load(context.Background())
		if err != nil {
			fmt.Println("Load error: ", err)
			return
		}

		fmt.Println("Sender schedule loaded")
	}()

	return nil
}

// Send serialized tx via QUIC
func sendTransaction(this js.Value, args []js.Value) interface{} {
	txBytes := make([]byte, args[0].Length())
	js.CopyBytesToGo(txBytes, args[0])

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := sender.Send(ctx, txBytes); err != nil {
			fmt.Println("Send error: ", err)
			return
		}
	}()

	return nil
}

// WASM entry point
func main() {
	js.Global().Set("initializeTransactionSender", js.FuncOf(load))
	js.Global().Set("sendTransaction", js.FuncOf(sendTransaction))
	select {}
}
