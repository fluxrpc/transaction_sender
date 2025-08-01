package solana_transaction_sender

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/quic-go/quic-go"
	"math/big"
	"net"
	"net/http"
	"time"
)

type TransactionSender struct {
	rpcEndpoint string
	http        *http.Client
	cert        tls.Certificate

	epochInfo      getEpochInfoResponse
	clusterNodes   getClusterNodesResponse
	leaderSchedule getLeaderScheduleResponse

	slotToLeader map[uint64]*leader
}

func NewTransactionSender(rpcEndpoint string) (*TransactionSender, error) {
	if rpcEndpoint == "" {
		return nil, errors.New("invalid rpc endpoint")
	}

	ts := TransactionSender{
		rpcEndpoint: rpcEndpoint,
		http:        &http.Client{Timeout: 5 * time.Second},
	}

	ts.cert, _ = ts.genSolanaCert()

	return &ts, nil
}

func (s *TransactionSender) Load(ctx context.Context) error {
	if err := s.getEpochInfo(ctx); err != nil {
		return err
	}

	if err := s.getLeaderSchedule(ctx); err != nil {
		return err
	}

	if err := s.getClusterNodes(ctx); err != nil {
		return err
	}

	s.buildSlotMap()

	return nil
}

func (s *TransactionSender) Send(ctx context.Context, txBytes []byte) error {
	dl := make(map[string]bool, 6)
	for i := uint64(0); i < 6; i++ {
		l, err := s.getLeader(ctx, i)
		if err != nil {
			fmt.Println("Leader fetch error:", err)
			return err
		}
		if _, ok := dl[l.PubKey]; ok {
			continue
		}
		dl[l.PubKey] = true

		go func() {
			cx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			tn := time.Now()
			err = s.sendLeader(cx, l, txBytes)
			if err != nil {
				fmt.Println(l.PubKey, " - ", l.TPUQuic, " - Leader send error:", err, " Took: ", time.Since(tn))
				return
			}

			fmt.Println(l.PubKey, " - ", l.TPUQuic, "Transaction sent via QUIC")
		}()
	}

	return nil
}

func (s *TransactionSender) sendLeader(ctx context.Context, l *leader, txBytes []byte) error {
	tn := time.Now()
	udpAddr, err := net.ResolveUDPAddr("udp", l.TPUQuic)
	if err != nil {
		fmt.Println("UDP resolve error:", err)
		return err
	}

	session, err := quic.DialAddr(ctx, udpAddr.String(), &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"solana-tpu"},
		GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &s.cert, nil
		},
	}, &quic.Config{})
	if err != nil {
		fmt.Println("QUIC dial error:", err)
		return err
	}
	fmt.Println("Dial took: ", time.Since(tn))
	tn = time.Now()

	defer session.CloseWithError(0, "done")

	stream, err := session.OpenUniStreamSync(ctx)
	if err != nil {
		fmt.Println("Stream accept error:", err)
		return err
	}
	defer stream.Close()

	_, err = stream.Write(txBytes)
	if err != nil {
		fmt.Println("Stream write error:", err)
		return err
	}
	return nil
}

func (s *TransactionSender) relativeSlot(slot uint64) uint64 {
	return slot - (s.epochInfo.Result.AbsoluteSlot - s.epochInfo.Result.SlotIndex)
}

func (s *TransactionSender) getLeader(ctx context.Context, slotDiff uint64) (*leader, error) {
	slot, err := s.getSlot(ctx)
	if err != nil {
		return nil, err
	}

	l := s.getLeaderAtSlot(s.relativeSlot(slot + slotDiff))
	if l == nil {
		return nil, fmt.Errorf("leader not found for slot %v - relative: %v", slot, s.relativeSlot(slot))
	}

	fmt.Println("Leader at slot ", slot+slotDiff, " - ", l.PubKey)
	return l, nil
}

func (s *TransactionSender) getLeaderAtSlot(slot uint64) *leader {
	return s.slotToLeader[slot]
}

func (s *TransactionSender) buildSlotMap() {
	lMap := make(map[string]*leader, len(s.clusterNodes.Result))
	for _, n := range s.clusterNodes.Result {
		lMap[n.PubKey] = n
	}

	s.slotToLeader = make(map[uint64]*leader)
	for l, slots := range s.leaderSchedule.Result {
		for _, slot := range slots {
			s.slotToLeader[slot] = lMap[l] //Bind addr to leader ref
		}
	}

}

func (s *TransactionSender) getSlot(ctx context.Context) (uint64, error) {
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

func (s *TransactionSender) getLeaderSchedule(ctx context.Context) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getLeaderSchedule",
		"params":  []any{nil, map[string]string{"commitment": "confirmed"}},
	})

	return s.rpcRequest(ctx, bytes.NewBuffer(body), &s.leaderSchedule)
}

func (s *TransactionSender) getClusterNodes(ctx context.Context) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getClusterNodes",
	})

	return s.rpcRequest(ctx, bytes.NewBuffer(body), &s.clusterNodes)
}

func (s *TransactionSender) getEpochInfo(ctx context.Context) error {
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getEpochInfo",
	})

	return s.rpcRequest(ctx, bytes.NewBuffer(body), &s.epochInfo)
}

func (s *TransactionSender) rpcRequest(ctx context.Context, reqData *bytes.Buffer, out any) error {
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

func (s *TransactionSender) genSolanaCert() (tls.Certificate, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "solana-node"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}
