package transaction_sender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type RPCService struct {
	rpcEndpoint string
	wsEndpoint  string
	http        *http.Client
}

func (s *RPCService) Load(rpcEndpoint, wsEndpoint string) error {
	if rpcEndpoint == "" {
		return errors.New("invalid rpc endpoint")
	}

	s.rpcEndpoint = rpcEndpoint
	s.wsEndpoint = wsEndpoint
	s.http = &http.Client{Timeout: 5 * time.Second}

	return nil
}

func (s *RPCService) Slot(ctx context.Context) (uint64, error) {
	var slotResult getSlotResponse
	err := s.rpcRequest(ctx, bytes.NewBuffer([]byte(`{"jsonrpc": "2.0","id": 1,"method": "getSlot","params": [{"commitment": "processed"}]}`)), &slotResult)
	if err != nil {
		return 0, err
	}

	return slotResult.Result, nil
}

func (s *RPCService) LeaderSchedule(ctx context.Context, epoch *uint64, out interface{}) error {
	var body string
	if epoch == nil {
		body = `{"jsonrpc": "2.0","id": 1,"method": "getLeaderSchedule"}`
	} else {
		body = fmt.Sprintf(`{"jsonrpc": "2.0","id": 1,"method": "getLeaderSchedule","params": [%v]}`, *epoch)
	}

	return s.rpcRequest(ctx, bytes.NewBuffer([]byte(body)), out)
}

func (s *RPCService) ClusterNodes(ctx context.Context, out interface{}) error {
	return s.rpcRequest(ctx, bytes.NewBuffer([]byte(`{"jsonrpc": "2.0","id": 1,"method": "getClusterNodes"}`)), out)
}

func (s *RPCService) EpochInfo(ctx context.Context, out interface{}) error {
	return s.rpcRequest(ctx, bytes.NewBuffer([]byte(`{"jsonrpc": "2.0","id": 1,"method": "getEpochInfo"}`)), out)
}

func (s *RPCService) rpcRequest(ctx context.Context, reqData *bytes.Buffer, out interface{}) error {
	req, _ := http.NewRequestWithContext(ctx, "POST", s.rpcEndpoint, reqData)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("%s", reqData.Bytes())
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, out)
	if err != nil {
		return err
	}

	return nil
}

func (s *RPCService) wsSlotSubscription(out chan<- uint64) error {
	conn, _, _, err := ws.DefaultDialer.Dial(context.Background(), s.wsEndpoint)
	if err != nil {
		log.Error().Err(err).Msg("RPCService::wsSlotSubscription connect error")
		return err
	}

	if err = wsutil.WriteClientMessage(conn, ws.OpText, []byte(`{"jsonrpc": "2.0","id": 1,"method": "slotSubscribe"}`)); err != nil {
		return err
	}

	go func() {
		defer conn.Close()
		for {
			data, _, err := wsutil.ReadServerData(conn)
			if err != nil {
				log.Error().Err(err).Msg("RPCService::wsSlotSubscription error")
				return
			}

			var msg SlotMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			select {
			case out <- msg.Params.Result.Slot:
			default:
				log.Warn().Msg("RPCService::wsSlotSubscription chan full")
			}
		}
	}()

	return nil
}
