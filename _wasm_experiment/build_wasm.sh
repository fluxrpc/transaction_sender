#!/bin/bash

GOOS=js GOARCH=wasm go build -o ../transaction_sender.wasm

echo "NOTE: WASM Does not work as no access to UDP sockets"