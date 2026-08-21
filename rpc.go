package transaction_sender

import (
	"context"
	"errors"
	"fmt"

	solanarpc "github.com/fluxrpc/solana-go/rpc"
	solanaws "github.com/fluxrpc/solana-go/ws"
	"github.com/rs/zerolog/log"
)

type RPCService struct {
	wsEndpoint string
	client     *solanarpc.Client
}

func (s *RPCService) Load(rpcEndpoint, wsEndpoint string) error {
	if rpcEndpoint == "" {
		return errors.New("invalid rpc endpoint")
	}
	if wsEndpoint == "" {
		return errors.New("invalid websocket endpoint")
	}

	s.wsEndpoint = wsEndpoint
	s.client = solanarpc.New(rpcEndpoint)
	return nil
}

func (s *RPCService) Slot(ctx context.Context) (uint64, error) {
	return s.client.GetSlot(ctx, solanarpc.CommitmentProcessed)
}

func (s *RPCService) LeaderSchedule(ctx context.Context, epoch *uint64, out **getLeaderScheduleResponse) error {
	result, err := s.client.GetLeaderScheduleWithOpts(ctx, &solanarpc.GetLeaderScheduleOpts{
		Commitment: solanarpc.CommitmentProcessed,
		Epoch:      epoch,
	})
	if err != nil {
		return err
	}

	schedule := make(map[string][]uint64, len(result))
	for identity, slots := range result {
		schedule[identity.String()] = slots
	}
	*out = &getLeaderScheduleResponse{Result: schedule}
	return nil
}

func (s *RPCService) ClusterNodes(ctx context.Context, out **getClusterNodesResponse) error {
	result, err := s.client.GetClusterNodes(ctx)
	if err != nil {
		return err
	}

	nodes := make([]*Leader, 0, len(result))
	for i := range result {
		node := &result[i]
		nodes = append(nodes, &Leader{
			PubKey:          node.Pubkey.String(),
			TPU:             s.stringValue(node.TPU),
			TPUForwards:     s.stringValue(node.TPUForwards),
			TPUQuic:         s.stringValue(node.TPUQUIC),
			TPUForwardsQuic: s.stringValue(node.TPUForwardsQUIC),
		})
	}
	*out = &getClusterNodesResponse{Result: nodes}
	return nil
}

func (s *RPCService) EpochInfo(ctx context.Context, out **getEpochInfoResponse) error {
	result, err := s.client.GetEpochInfo(ctx, solanarpc.CommitmentProcessed)
	if err != nil {
		return err
	}
	*out = &getEpochInfoResponse{Result: epochInfo{
		Epoch:        result.Epoch,
		SlotIndex:    result.SlotIndex,
		AbsoluteSlot: result.AbsoluteSlot,
		SlotsInEpoch: result.SlotsInEpoch,
	}}
	return nil
}

func (s *RPCService) wsSlotSubscription(out chan<- uint64) error {
	client, err := solanaws.Connect(context.Background(), s.wsEndpoint)
	if err != nil {
		return fmt.Errorf("connect slot websocket: %w", err)
	}
	subscription, err := client.SlotSubscribe(context.Background())
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("subscribe to slots: %w", err)
	}

	go s.receiveSlots(client, subscription, out)
	return nil
}

func (s *RPCService) receiveSlots(client *solanaws.Client, subscription *solanaws.Subscription[solanaws.SlotResult], out chan<- uint64) {
	defer client.Close()
	for {
		update, err := subscription.Recv(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("slot subscription stopped")
			return
		}
		select {
		case out <- update.Slot:
		default:
			log.Warn().Msg("slot subscription channel full")
		}
	}
}

func (s *RPCService) stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
