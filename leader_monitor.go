package transaction_sender

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"time"
)

type LeaderMonitor struct {
	rpc *RPCService

	epochInfo      getEpochInfoResponse
	clusterNodes   getClusterNodesResponse
	leaderSchedule getLeaderScheduleResponse

	slotToLeader map[uint64]*Leader
}

func (s *LeaderMonitor) Start() error {
	return s.load()
}

func (s *LeaderMonitor) load() error {
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

func (s *LeaderMonitor) dataWorker() {
	for {
		time.Sleep(20 * time.Minute)
		err := s.load() //Load new data
		if err != nil {
			log.Error().Err(err).Msg("LeaderMonitor::dataWorker error")
		}

	}
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

	l := s.getLeaderAtSlot(s.RelativeSlot(slot + slotDiff))
	if l == nil {
		return nil, fmt.Errorf("leader not found for slot %v - relative: %v", slot, s.RelativeSlot(slot))
	}

	log.Debug().Str("pk", l.PubKey).Uint64("slot", slot+slotDiff).Msg("LeaderMonitor::Current Leader")
	return l, nil
}

func (s *LeaderMonitor) RelativeSlot(slot uint64) uint64 {
	return slot - (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex)
}
