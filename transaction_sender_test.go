package transaction_sender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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
