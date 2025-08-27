package transaction_sender

import (
	"context"
	"errors"
	"github.com/rs/zerolog/log"
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
	l, leaderSlot, err := s.leader.Current(1) // N+1
	if err != nil {
		log.Error().Err(err).Msg("TransactionSender::Send error")
		return err
	}

	log.Info().Str("leader", l.PubKey).Uint64("slot", leaderSlot).Msg("Sending Txn")
	return s.tpu.Send(ctx, l, txBytes)
}
