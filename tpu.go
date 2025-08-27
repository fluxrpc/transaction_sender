package transaction_sender

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
	"math/big"
	"net"
	"sync"
	"time"
)

type connectedLeader struct {
	Addr        string
	Conn        *quic.Conn
	Stream      *quic.SendStream
	ConnectedAt time.Time
}

func (c *connectedLeader) Expired() bool {
	return time.Since(c.ConnectedAt) > 5*time.Second // TPU Timeout
}

type TPUService struct {
	cert tls.Certificate

	cachedConns map[string]*connectedLeader
	muCache     *sync.RWMutex

	connecting map[string]struct{}
	muConnect  *sync.RWMutex
}

func (s *TPUService) Start() error {
	s.cachedConns = map[string]*connectedLeader{}
	s.muCache = &sync.RWMutex{}

	s.connecting = map[string]struct{}{}
	s.muConnect = &sync.RWMutex{}

	s.cert, _ = s.genSolanaCert()

	go s.tidyCachedConnsWorker()

	return nil
}

func (s *TPUService) Send(ctx context.Context, l *Leader, txBytes []byte) error {
	_ = s._sendUDP(ctx, l, txBytes)
	_ = s._sendQUIC(ctx, l, txBytes)
	return nil
}

func (s *TPUService) Connect(ctx context.Context, leader *Leader) (*quic.Conn, error) {
	//if c := s.cachedConn(quicEndpoint); c != nil {
	//	return c, nil
	//}

	if s.isConnecting(leader.TPUQuic) {
		return nil, errors.New("connection not ready")
	}

	log.Debug().Str("quic", leader.TPUQuic).Msgf("TPUService::Connect Connecting")
	return s.connect(ctx, leader)
}

func (s *TPUService) PreConnect(ctx context.Context, l *Leader) error {
	if c := s.cachedConn(l.TPUQuic); c != nil || s.isConnecting(l.TPUQuic) {
		return nil
	}

	log.Debug().Str("quic", l.TPUQuic).Str("leader", l.PubKey).Msgf("TPUService::PreConnect Connecting")
	_, err := s.connect(ctx, l)
	return err
}

func (s *TPUService) connect(ctx context.Context, l *Leader) (*quic.Conn, error) {
	s.setConnecting(l.TPUQuic, true)
	defer s.setConnecting(l.TPUQuic, false)

	tn := time.Now()
	udpAddr, err := net.ResolveUDPAddr("udp", l.TPUQuic)
	if err != nil {
		log.Error().Err(err).Str("quic", l.TPUQuic).Str("leader", l.PubKey).Msg("TPUService::PreConnect error")
		return nil, err
	}

	conn, err := quic.DialAddr(ctx, udpAddr.String(), &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"solana-tpu"},
		GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &s.cert, nil
		},
	}, &quic.Config{})
	if err != nil {
		log.Error().Err(err).Str("quic", l.TPUQuic).Str("leader", l.PubKey).Msg("TPUService::PreConnect dial error")
		return nil, err
	}

	log.Info().Str("quic", l.TPUQuic).Str("leader", l.PubKey).Msgf("TPUService::Connect Dial took: %s", time.Since(tn))

	stream, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::PreConnect accept error")
		return nil, err
	}

	_, err = stream.Write([]byte{})
	if err != nil {
		log.Error().Err(err).Msg("TPUService::PreConnect write error")
		return nil, err
	}

	s.cacheConn(l.TPUQuic, conn, stream)
	return conn, nil
}

func (s *TPUService) _sendQUIC(ctx context.Context, l *Leader, txBytes []byte) error {
	conn, err := s.Connect(ctx, l)
	if err != nil {
		return err
	}

	stream, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::_sendQUIC accept error")
		return err
	}
	defer stream.Close()

	_, err = stream.Write(txBytes)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::_sendQUIC write error")
		return err
	}
	return nil
}

func (s *TPUService) _sendUDP(ctx context.Context, leader *Leader, txBytes []byte) error {
	addr, err := net.ResolveUDPAddr("udp", leader.TPU)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::_sendUDP dial error")
		return err
	}
	defer conn.Close()

	// Short write deadline so we don't hang.
	_ = conn.SetWriteDeadline(time.Now().Add(400 * time.Millisecond))

	_, err = conn.Write(txBytes)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::_sendUDP write error")
		return err
	}

	return nil
}

func (s *TPUService) cacheConn(quicEndpoint string, conn *quic.Conn, stream *quic.SendStream) {
	s.muCache.Lock()
	defer s.muCache.Unlock()
	s.cachedConns[quicEndpoint] = &connectedLeader{
		Addr:        quicEndpoint,
		Conn:        conn,
		Stream:      stream,
		ConnectedAt: time.Now(),
	}
}

func (s *TPUService) cachedConn(quicEndpoint string) *quic.Conn {
	s.muCache.RLock()
	defer s.muCache.RUnlock()
	if c, ok := s.cachedConns[quicEndpoint]; ok && !c.Expired() {
		log.Debug().Msgf("TPUService::cachedConn - connectedSince: %s", time.Since(c.ConnectedAt))
		return c.Conn
	}

	return nil
}

func (s *TPUService) isConnecting(quicEndpoint string) bool {
	s.muConnect.RLock()
	defer s.muConnect.RUnlock()
	_, ok := s.connecting[quicEndpoint]
	return ok
}

func (s *TPUService) setConnecting(quicEndpoint string, connecting bool) {
	s.muConnect.Lock()
	defer s.muConnect.Unlock()
	if connecting {
		s.connecting[quicEndpoint] = struct{}{}
	} else {
		delete(s.connecting, quicEndpoint)
	}
}

func (s *TPUService) tidyCachedConnsWorker() {
	for {
		time.Sleep(5 * time.Second)
		s.tidyCachedConns()
	}
}

func (s *TPUService) tidyCachedConns() {
	s.muCache.Lock()
	defer s.muCache.Unlock()
	for qe, cl := range s.cachedConns {
		if cl.Expired() {
			_ = cl.Stream.Close()
			_ = cl.Conn.CloseWithError(0, "done")
			delete(s.cachedConns, qe)
		}
	}
}

func (s *TPUService) genSolanaCert() (tls.Certificate, error) {
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
