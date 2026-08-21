package transaction_sender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRPCServiceUsesSolanaGoClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}

		var result string
		switch request.Method {
		case "getSlot":
			result = `123`
		case "getEpochInfo":
			result = `{"absoluteSlot":123,"blockHeight":100,"epoch":2,"slotIndex":23,"slotsInEpoch":100}`
		case "getClusterNodes":
			result = `[{"pubkey":"11111111111111111111111111111111","tpu":"127.0.0.1:8001","tpuQuic":"127.0.0.1:8002"}]`
		case "getLeaderSchedule":
			result = `{"11111111111111111111111111111111":[0,1]}`
		default:
			t.Errorf("unexpected RPC method %q", request.Method)
			return
		}
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, result)
	}))
	defer server.Close()

	service := &RPCService{}
	if err := service.Load(server.URL, "ws://127.0.0.1:8900"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	slot, err := service.Slot(ctx)
	if err != nil || slot != 123 {
		t.Fatalf("Slot() = %d, %v; want 123, nil", slot, err)
	}

	var epoch *getEpochInfoResponse
	if err := service.EpochInfo(ctx, &epoch); err != nil {
		t.Fatal(err)
	}
	if epoch.Result.Epoch != 2 || epoch.Result.SlotIndex != 23 {
		t.Fatalf("unexpected epoch info: %#v", epoch.Result)
	}

	var nodes *getClusterNodesResponse
	if err := service.ClusterNodes(ctx, &nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes.Result) != 1 || nodes.Result[0].TPUQuic != "127.0.0.1:8002" {
		t.Fatalf("unexpected cluster nodes: %#v", nodes.Result)
	}

	var schedule *getLeaderScheduleResponse
	if err := service.LeaderSchedule(ctx, nil, &schedule); err != nil {
		t.Fatal(err)
	}
	if len(schedule.Result["11111111111111111111111111111111"]) != 2 {
		t.Fatalf("unexpected leader schedule: %#v", schedule.Result)
	}
}

func TestRPCServiceRejectsMissingEndpoints(t *testing.T) {
	service := &RPCService{}
	if err := service.Load("", "ws://127.0.0.1:8900"); err == nil {
		t.Fatal("expected empty RPC endpoint to fail")
	}
	if err := service.Load("http://127.0.0.1:8899", ""); err == nil {
		t.Fatal("expected empty websocket endpoint to fail")
	}
}

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
