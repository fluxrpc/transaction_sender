package transaction_sender

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"time"
)

type LeaderMonitor struct {
	rpc *RPCService

	onUpcomingLeader func(currentSlot uint64, leaderSlot uint64, leader *Leader)

	epochInfo    *getEpochInfoResponse
	clusterNodes *getClusterNodesResponse

	leaderSchedule     *getLeaderScheduleResponse
	nextLeaderSchedule *getLeaderScheduleResponse

	slotToLeader map[uint64]*Leader

	nextLeaderG *Leader

	onSlot      chan uint64
	currentSlot uint64
}

func (s *LeaderMonitor) Start() error {
	tn := time.Now()

	err := s.load()
	if err != nil {
		return err
	}
	log.Debug().Msgf("LeaderMonitor::load took: %s", time.Since(tn))

	s.onSlot = make(chan uint64, 10)
	err = s.rpc.wsSlotSubscription(s.onSlot)
	if err != nil {
		return err
	}

	go s.slotWorker()

	return nil
}

func (s *LeaderMonitor) rotateSchedule() error {

	s.leaderSchedule = s.nextLeaderSchedule
	s.buildSlotMap()

	go s.load() //Async fetch data for next epoch

	return nil
}

func (s *LeaderMonitor) load() error {
	ctx := context.TODO()

	if err := s.rpc.EpochInfo(ctx, &s.epochInfo); err != nil {
		return err
	}
	log.Info().Uint64("epoch", s.epochInfo.Result.Epoch).Msg("Epoch Loaded")

	if err := s.rpc.ClusterNodes(ctx, &s.clusterNodes); err != nil {
		return err
	}
	log.Info().Int("node_count", len(s.clusterNodes.Result)).Msg("ClusterNodes Loaded")

	//Only load leader schedule once as it is populated from nextLeaderSchedule on rotation of epoch
	if s.leaderSchedule == nil {
		if err := s.rpc.LeaderSchedule(ctx, nil, &s.leaderSchedule); err != nil {
			return err
		}
		log.Info().Uint64("epoch", s.epochInfo.Result.Epoch).Int("leader_count", len(s.leaderSchedule.Result)).Msg("LeaderSchedule Loaded")
	}

	nextEpoch := s.epochInfo.Result.AbsoluteSlot + s.epochInfo.Result.SlotsInEpoch
	if err := s.rpc.LeaderSchedule(ctx, &nextEpoch, &s.nextLeaderSchedule); err != nil {
		return err
	}
	log.Info().Uint64("epoch", s.epochInfo.Result.Epoch+1).Int("leader_count", len(s.nextLeaderSchedule.Result)).Msg("Next LeaderSchedule Loaded")

	s.buildSlotMap()
	return nil
}

func (s *LeaderMonitor) slotWorker() {
	for slot := range s.onSlot {
		s.currentSlot = s.RelativeSlot(slot)
		if s.currentSlot == 0 {
			log.Debug().Msg("LeaderMonitor::slotWorker ROTATE_EPOCH")
			_ = s.rotateSchedule()
		}

		atSlot, nextLeader := s.nextLeader(s.currentSlot)
		if nextLeader == nil {
			continue
		}

		if s.nextLeaderG != nil && nextLeader.PubKey == s.nextLeaderG.PubKey {
			if s.onUpcomingLeader != nil {
				go s.onUpcomingLeader(s.currentSlot, atSlot, nextLeader)
			}
			continue
		}
		// New Leader

		s.nextLeaderG = nextLeader

		log.Trace().Uint64("slot", s.currentSlot).Uint64("leader_slot", atSlot).Str("pk", nextLeader.PubKey).Msgf("LeaderMonitor::slotWorker NextLeader in: %v", atSlot-s.currentSlot)
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

func (s *LeaderMonitor) nextLeader(slot uint64) (uint64, *Leader) {
	current := s.slotToLeader[slot]
	if current == nil {
		return 0, nil
	}

	for i := slot; i < uint64(len(s.slotToLeader)); i++ {
		l := s.slotToLeader[i]
		if l == nil {
			log.Warn().Uint64("slot", i).Msg("Leader not found in slot map")
			continue
		}

		if l.PubKey != current.PubKey {
			return i, l
		}
	}

	for ls, l := range s.slotToLeader {
		if l.PubKey != current.PubKey {
			return ls, l
		}
	}
	return 0, nil
}

func (s *LeaderMonitor) Current(slotDiff uint64) (*Leader, uint64, error) {
	l := s.getLeaderAtSlot(s.currentSlot + slotDiff)
	if l == nil {
		return nil, 0, fmt.Errorf("leader not found for slot %v", s.currentSlot+slotDiff)
	}

	return l, s.currentSlot + slotDiff + (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex), nil
}

func (s *LeaderMonitor) RelativeSlot(slot uint64) uint64 {
	return slot - (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex)
}
