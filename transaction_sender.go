package transaction_sender

import (
	"context"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"time"
)

type TransactionSender struct {
	leader *LeaderMonitor
	tpu    *TPUService
}

func NewTransactionSender(rpcEndpoint string, websocketEndpoint string) (*TransactionSender, error) {
	if rpcEndpoint == "" {
		return nil, errors.New("invalid rpc endpoint")
	}

	rpc := &RPCService{}
	if err := rpc.Load(rpcEndpoint, websocketEndpoint); err != nil {
		return nil, err
	}

	tpu := &TPUService{}
	if err := tpu.Start(); err != nil {
		return nil, err
	}

	lm := &LeaderMonitor{
		rpc: rpc,
		onUpcomingLeader: func(currentSlot uint64, leaderSlot uint64, leader *Leader) {
			if leaderSlot-currentSlot > 2 {
				return //Ignore anything far out
			}

			if err := tpu.PreConnect(context.TODO(), leader); err != nil {
				log.Error().Err(err).Str("leader", leader.PubKey).Msg("Preconnect leader failed")
			}
		},
	}
	if err := lm.Start(); err != nil {
		return nil, err
	}

	ts := TransactionSender{
		leader: lm,
		tpu:    tpu,
	}

	return &ts, nil
}

func (s *TransactionSender) Send(ctx context.Context, txBytes []byte) error {
	targets := s.leader.sendTargets(s.tpu, time.Now())
	if len(targets) == 0 {
		err := fmt.Errorf("leader not found for current or next slot")
		log.Error().Err(err).Msg("TransactionSender::Send error")
		return err
	}

	for _, l := range targets {
		log.Info().Str("leader", l.PubKey).Msg("Sending Txn")
		_ = s.tpu.Send(ctx, l, txBytes)
	}
	return nil
}
