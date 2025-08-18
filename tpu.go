package transaction_sender

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
	"math/big"
	"net"
	"time"
)

type connectedLeader struct {
	Addr string
	Conn *quic.Conn
}

type TPUService struct {
	cert tls.Certificate

	connectedLeader *connectedLeader
}

func (s *TPUService) Start() error {
	s.cert, _ = s.genSolanaCert()
	return nil
}

func (s *TPUService) Send(ctx context.Context, l *Leader, txBytes []byte) error {
	_ = s._sendUDP(ctx, l.TPU, txBytes)
	_ = s._sendQUIC(ctx, l.TPUQuic, txBytes)
	return nil
}

func (s *TPUService) PreConnect(ctx context.Context, quicEndpoint string) (*quic.Conn, error) {
	if s.connectedLeader != nil && s.connectedLeader.Addr == quicEndpoint {
		return s.connectedLeader.Conn, nil
	}

	tn := time.Now()
	udpAddr, err := net.ResolveUDPAddr("udp", quicEndpoint)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::PreConnect error")
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
		log.Error().Err(err).Msg("TPUService::PreConnect dial error")
		return nil, err
	}

	log.Trace().Msgf("Dial took: %s", time.Since(tn))
	return conn, nil
}

func (s *TPUService) _sendQUIC(ctx context.Context, quicEndpoint string, txBytes []byte) error {
	conn, err := s.PreConnect(ctx, quicEndpoint)
	if err != nil {
		return err
	}

	defer conn.CloseWithError(0, "done")

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

func (s *TPUService) _sendUDP(ctx context.Context, udpEndpoint string, txBytes []byte) error {
	addr, err := net.ResolveUDPAddr("udp", udpEndpoint)
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
	_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))

	_, err = conn.Write(txBytes)
	if err != nil {
		log.Error().Err(err).Msg("TPUService::_sendUDP write error")
		return err
	}

	return nil
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
