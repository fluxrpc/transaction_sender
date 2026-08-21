package transaction_sender

import (
	"context"
	"fmt"
	"github.com/rs/zerolog/log"
	"sync"
	"time"
)

const (
	defaultSlotDuration = 200 * time.Millisecond
	rotationGuard       = 25 * time.Millisecond
	defaultRTT          = 100 * time.Millisecond
	minSlotDuration     = 25 * time.Millisecond
	maxSlotDuration     = 2 * time.Second
)

type LeaderMonitor struct {
	rpc *RPCService

	onUpcomingLeader func(currentSlot uint64, leaderSlot uint64, leader *Leader)

	epochInfo    *getEpochInfoResponse
	clusterNodes *getClusterNodesResponse

	leaderSchedule     *getLeaderScheduleResponse
	nextLeaderSchedule *getLeaderScheduleResponse

	slotToLeader map[uint64]*Leader
	muSchedule   sync.RWMutex

	nextLeaderG *Leader

	onSlot        chan uint64
	currentSlot   uint64
	slotStartedAt time.Time

	muSlot                sync.RWMutex
	lastAbsoluteSlot      uint64
	hasSlotObservation    bool
	estimatedSlotDuration time.Duration
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
		currentSlot := s.RelativeSlot(slot)
		s.observeSlotTiming(slot, currentSlot, time.Now())
		if currentSlot == 0 {
			log.Debug().Msg("LeaderMonitor::slotWorker ROTATE_EPOCH")
			_ = s.rotateSchedule()
		}

		atSlot, nextLeader := s.nextLeader(currentSlot)
		if nextLeader == nil {
			continue
		}

		if s.nextLeaderG != nil && nextLeader.PubKey == s.nextLeaderG.PubKey {
			if s.onUpcomingLeader != nil {
				go s.onUpcomingLeader(currentSlot, atSlot, nextLeader)
			}
			continue
		}
		// New Leader

		s.nextLeaderG = nextLeader

		log.Trace().Uint64("slot", currentSlot).Uint64("leader_slot", atSlot).Str("pk", nextLeader.PubKey).Msgf("LeaderMonitor::slotWorker NextLeader in: %v", atSlot-currentSlot)
	}
}

func (s *LeaderMonitor) observeSlotTiming(absoluteSlot uint64, relativeSlot uint64, observedAt time.Time) {
	s.muSlot.Lock()
	defer s.muSlot.Unlock()

	if s.hasSlotObservation && absoluteSlot <= s.lastAbsoluteSlot {
		return
	}
	if s.hasSlotObservation {
		delta := absoluteSlot - s.lastAbsoluteSlot
		sample := observedAt.Sub(s.slotStartedAt) / time.Duration(delta)
		if sample >= minSlotDuration && sample <= maxSlotDuration {
			if s.estimatedSlotDuration == 0 {
				s.estimatedSlotDuration = sample
			} else {
				s.estimatedSlotDuration = (4*s.estimatedSlotDuration + sample) / 5
			}
		}
	}

	s.currentSlot = relativeSlot
	s.slotStartedAt = observedAt
	s.lastAbsoluteSlot = absoluteSlot
	s.hasSlotObservation = true
}

func (s *LeaderMonitor) getLeaderAtSlot(slot uint64) *Leader {
	s.muSchedule.RLock()
	defer s.muSchedule.RUnlock()
	return s.slotToLeader[slot]
}

func (s *LeaderMonitor) leadersAtSlots(currentSlot uint64) (*Leader, *Leader) {
	s.muSchedule.RLock()
	defer s.muSchedule.RUnlock()
	return s.slotToLeader[currentSlot], s.slotToLeader[currentSlot+1]
}

func (s *LeaderMonitor) buildSlotMap() {
	lMap := make(map[string]*Leader, len(s.clusterNodes.Result))
	for _, n := range s.clusterNodes.Result {
		lMap[n.PubKey] = n
	}

	slotToLeader := make(map[uint64]*Leader)
	for l, slots := range s.leaderSchedule.Result {
		for _, slot := range slots {
			slotToLeader[slot] = lMap[l] //Bind addr to Leader ref
		}
	}
	s.muSchedule.Lock()
	s.slotToLeader = slotToLeader
	s.muSchedule.Unlock()
}

func (s *LeaderMonitor) nextLeader(slot uint64) (uint64, *Leader) {
	s.muSchedule.RLock()
	defer s.muSchedule.RUnlock()
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
	s.muSlot.RLock()
	currentSlot := s.currentSlot
	s.muSlot.RUnlock()
	l := s.getLeaderAtSlot(currentSlot + slotDiff)
	if l == nil {
		return nil, 0, fmt.Errorf("leader not found for slot %v", currentSlot+slotDiff)
	}

	return l, currentSlot + slotDiff + (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex), nil
}

// sendTargets returns the leaders to send to right now: always the N+1 leader,
// plus the current leader while half its RTT still fits in the slot's remaining time.
func (s *LeaderMonitor) sendTargets(tpu *TPUService, now time.Time) []*Leader {
	s.muSlot.RLock()
	currentSlot := s.currentSlot
	slotStartedAt := s.slotStartedAt
	slotDuration := s.estimatedSlotDuration
	s.muSlot.RUnlock()
	if slotDuration == 0 {
		slotDuration = defaultSlotDuration
	}

	current, next := s.leadersAtSlots(currentSlot)
	if next == nil {
		if current == nil {
			return nil
		}
		return []*Leader{current}
	}
	if current == nil || current.PubKey == next.PubKey {
		return []*Leader{next}
	}

	remaining := slotDuration - now.Sub(slotStartedAt)
	if tpu.RTT(current)/2+rotationGuard < remaining {
		return []*Leader{current, next}
	}
	return []*Leader{next}
}

func (s *LeaderMonitor) RelativeSlot(slot uint64) uint64 {
	return slot - (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex)
}
