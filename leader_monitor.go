package solana_transaction_sender

import (
	"context"
	"fmt"
)

type LeaderMonitor struct {
	rpc *RPCService

	epochInfo      getEpochInfoResponse
	clusterNodes   getClusterNodesResponse
	leaderSchedule getLeaderScheduleResponse

	slotToLeader map[uint64]*Leader
}

func (s *LeaderMonitor) Start() error {
	ctx := context.TODO()

	if err := s.rpc.EpochInfo(ctx, &s.epochInfo); err != nil {
		return err
	}

	if err := s.rpc.LeaderSchedule(ctx, &s.leaderSchedule); err != nil {
		return err
	}

	if err := s.rpc.ClusterNodes(ctx, &s.clusterNodes); err != nil {
		return err
	}

	s.buildSlotMap()
	return nil
}

func (s *LeaderMonitor) getLeaderAtSlot(slot uint64) *Leader {
	return s.slotToLeader[slot]
}

func (s *LeaderMonitor) buildSlotMap() {
	lMap := make(map[string]*Leader, len(s.clusterNodes.Result))
	for _, n := range s.clusterNodes.Result {
		lMap[n.PubKey] = n
	}

	s.slotToLeader = make(map[uint64]*Leader)
	for l, slots := range s.leaderSchedule.Result {
		for _, slot := range slots {
			s.slotToLeader[slot] = lMap[l] //Bind addr to Leader ref
		}
	}

}

func (s *LeaderMonitor) Current(ctx context.Context, slotDiff uint64) (*Leader, error) {
	slot, err := s.rpc.Slot(ctx)
	if err != nil {
		return nil, err
	}

	l := s.getLeaderAtSlot(s.rpc.RelativeSlot(slot + slotDiff))
	if l == nil {
		return nil, fmt.Errorf("leader not found for slot %v - relative: %v", slot, s.rpc.RelativeSlot(slot))
	}

	fmt.Println("Leader at slot ", slot+slotDiff, " - ", l.PubKey)
	return l, nil
}
