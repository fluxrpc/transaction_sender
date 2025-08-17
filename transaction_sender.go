package solana_transaction_sender

import (
	"context"
	"errors"
	"fmt"
)

type TransactionSender struct {
	leader *LeaderMonitor
	tpu    *TPUService
}

func NewTransactionSender(rpcEndpoint string) (*TransactionSender, error) {
	if rpcEndpoint == "" {
		return nil, errors.New("invalid rpc endpoint")
	}

	rpc := &RPCService{}
	if err := rpc.Load(rpcEndpoint); err != nil {
		return nil, err
	}

	tpu := &TPUService{}
	if err := tpu.Start(); err != nil {
		return nil, err
	}

	ts := TransactionSender{
		leader: &LeaderMonitor{
			rpc: rpc,
		},
		tpu: tpu,
	}

	return &ts, nil
}

func (s *TransactionSender) Send(ctx context.Context, txBytes []byte) error {
	l, err := s.leader.Current(ctx, 1) // N+1
	if err != nil {
		fmt.Println("Leader fetch error:", err)
		return err
	}

	return s.tpu.Send(ctx, l, txBytes)
}
