package transaction_sender

import (
	"testing"
	"time"
)

func TestSendTargetsIncludesCurrentLeaderWhenReachable(t *testing.T) {
	started := time.Unix(100, 0)
	current := &Leader{PubKey: "A", TPUQuic: "a:8001"}
	next := &Leader{PubKey: "B"}
	lm := &LeaderMonitor{
		slotToLeader:  map[uint64]*Leader{10: current, 11: next},
		currentSlot:   10,
		slotStartedAt: started,
	}
	tpu := &TPUService{rtts: map[string]time.Duration{"a:8001": 50 * time.Millisecond}}

	targets := lm.sendTargets(tpu, started)
	if len(targets) != 2 || targets[0] != current || targets[1] != next {
		t.Fatalf("targets = %v, want [current, next]", targets)
	}
}

func TestSendTargetsSkipsCurrentLeaderWhenRTTMissesSlot(t *testing.T) {
	started := time.Unix(100, 0)
	// ~30ms left in the slot but half of a 100ms RTT + guard needs 75ms
	lm := &LeaderMonitor{
		slotToLeader:  map[uint64]*Leader{10: {PubKey: "A", TPUQuic: "a:8001"}, 11: {PubKey: "B"}},
		currentSlot:   10,
		slotStartedAt: started,
	}
	tpu := &TPUService{rtts: map[string]time.Duration{"a:8001": 100 * time.Millisecond}}

	targets := lm.sendTargets(tpu, started.Add(defaultSlotDuration-30*time.Millisecond))
	if len(targets) != 1 || targets[0].PubKey != "B" {
		t.Fatalf("targets = %v, want only next leader", targets)
	}
}

func TestSendTargetsDedupesConsecutiveSlotsOfSameLeader(t *testing.T) {
	started := time.Unix(100, 0)
	leader := &Leader{PubKey: "A"}
	lm := &LeaderMonitor{
		slotToLeader:  map[uint64]*Leader{10: leader, 11: leader},
		currentSlot:   10,
		slotStartedAt: started,
	}

	targets := lm.sendTargets(&TPUService{}, started)
	if len(targets) != 1 || targets[0] != leader {
		t.Fatalf("targets = %v, want single leader", targets)
	}
}

func TestSlotDurationLearnsFromObservations(t *testing.T) {
	lm := &LeaderMonitor{}
	started := time.Unix(100, 0)
	lm.observeSlotTiming(100, 10, started)
	lm.observeSlotTiming(101, 11, started.Add(220*time.Millisecond))
	// Three slots over 600ms contributes a 200ms sample.
	lm.observeSlotTiming(104, 14, started.Add(820*time.Millisecond))

	if got := lm.estimatedSlotDuration; got != 216*time.Millisecond {
		t.Fatalf("slot duration = %s, want 216ms", got)
	}
}

func TestRTTSmoothsHandshakeSamples(t *testing.T) {
	s := &TPUService{rtts: map[string]time.Duration{}}
	s.observeRTT("1.2.3.4:8001", 100*time.Millisecond)
	s.observeRTT("1.2.3.4:8001", 200*time.Millisecond)

	if got := s.RTT(&Leader{TPUQuic: "1.2.3.4:8001"}); got != 120*time.Millisecond {
		t.Fatalf("RTT = %s, want 120ms", got)
	}
	if got := s.RTT(&Leader{TPUQuic: "5.6.7.8:8001"}); got != defaultRTT {
		t.Fatalf("RTT = %s, want defaultRTT for unmeasured leader", got)
	}
}
