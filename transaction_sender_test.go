package transaction_sender

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestNewTransactionSender(t *testing.T) {

}

func TestTransactionSender_getClusterNodes(t *testing.T) {
	s := &TransactionSender{
		rpcEndpoint: os.Getenv("RPC_URL"),
		http:        &http.Client{Timeout: 3 * time.Second},
	}
	err := s.getClusterNodes(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	if len(s.clusterNodes.Result) == 0 {
		t.Fail()
	}
}

func TestTransactionSender_getLeaderSchedule(t *testing.T) {
	s := &TransactionSender{
		rpcEndpoint: os.Getenv("RPC_URL"),
		http:        &http.Client{Timeout: 3 * time.Second},
	}
	err := s.getLeaderSchedule(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	if len(s.leaderSchedule.Result) == 0 {
		t.Fail()
	}
}

func TestTransactionSender_getSlot(t *testing.T) {
	s := &TransactionSender{
		rpcEndpoint: os.Getenv("RPC_URL"),
		http:        &http.Client{Timeout: 3 * time.Second},
	}
	slotResp, err := s.getSlot(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	if slotResp == 0 {
		t.Fail()
	}
}
