package solana_transaction_sender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type RPCService struct {
	rpcEndpoint string
	http        *http.Client
}

func (s *RPCService) Load(rpcEndpoint string) error {
	if rpcEndpoint == "" {
		return errors.New("invalid rpc endpoint")
	}

	s.rpcEndpoint = rpcEndpoint
	s.http = &http.Client{Timeout: 5 * time.Second}

	return nil
}

func (s *RPCService) Slot(ctx context.Context) (uint64, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSlot",
		"params":  []any{map[string]string{"commitment": "processed"}},
	})

	var slotResult getSlotResponse
	err := s.rpcRequest(ctx, bytes.NewBuffer(body), &slotResult)
	if err != nil {
		return 0, err
	}

	return slotResult.Result, nil
}

func (s *RPCService) LeaderSchedule(ctx context.Context, out interface{}) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getLeaderSchedule",
		"params":  []any{nil, map[string]string{"commitment": "confirmed"}},
	})

	return s.rpcRequest(ctx, bytes.NewBuffer(body), &s.leaderSchedule)
}

func (s *RPCService) ClusterNodes(ctx context.Context, out interface{}) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getClusterNodes",
	})

	return s.rpcRequest(ctx, bytes.NewBuffer(body), &s.clusterNodes)
}

func (s *RPCService) EpochInfo(ctx context.Context, out interface{}) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getEpochInfo",
	})

	return s.rpcRequest(ctx, bytes.NewBuffer(body), &s.epochInfo)
}

func (s *RPCService) rpcRequest(ctx context.Context, reqData *bytes.Buffer, out any) error {
	req, _ := http.NewRequestWithContext(ctx, "POST", s.rpcEndpoint, reqData)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}

	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return err
	}

	return nil
}

func (s *RPCService) RelativeSlot(slot uint64) uint64 {
	return slot - (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex)
}
