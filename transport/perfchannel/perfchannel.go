// Package perfchannel is a channel-based transport implementation
// that doesn't save incoming/outgoing packets and maximizes performance
// exclusively for the purpose of benchmark tests.
package perfchannel

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.dedis.ch/cs438/internal/traffic"
	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

var counter uint32 // initialized by default to 0

// NewTransport returns a channel-based transport service.
func NewTransport() transport.Transport {
	return &PerfTransport{
		incomings: make(map[string]chan transport.Packet),
		traffic:   traffic.NewTraffic(),
	}
}

// PerfTransport is a simple transport implementation using channels
//
// - implements transport.Transport
// - implements transport.ClosableSocket
type PerfTransport struct {
	sync.RWMutex
	incomings map[string]chan transport.Packet
	traffic   *traffic.Traffic
}

// CreateSocket implements transport.Transport
func (t *PerfTransport) CreateSocket(address string) (transport.ClosableSocket, error) {
	t.Lock()
	if strings.HasSuffix(address, ":0") {
		address = address[:len(address)-2]
		port := atomic.AddUint32(&counter, 1)
		address = fmt.Sprintf("%s:%d", address, port)
	}
	t.incomings[address] = make(chan transport.Packet, 100)
	t.Unlock()

	return &PerfSocket{
		PerfTransport: t,
		myAddr:        address,
	}, nil
}

// MustCreate returns a socket and panic if something goes wrong. This function
// is mostly usefull in tests.
func (t *PerfTransport) MustCreate(address string) transport.ClosableSocket {
	socket, err := t.CreateSocket(address)
	if err != nil {
		panic("failed to create socket: " + err.Error())
	}

	return socket
}

// PerfSocket provide a performance-focused network layer using channels.
//
// - implements transport.Socket
type PerfSocket struct {
	*PerfTransport
	myAddr string
}

// Close implements transport.Socket
func (s *PerfSocket) Close() error {
	s.Lock()
	defer s.Unlock()

	delete(s.incomings, s.myAddr)

	return nil
}

// Send implements transport.Socket.
func (s *PerfSocket) Send(dest string, pkt transport.Packet, timeout time.Duration) error {
	s.RLock()
	to, ok := s.incomings[dest]

	if !ok {
		s.RUnlock()
		return xerrors.Errorf("%s is not listening", dest)
	}
	s.RUnlock()

	if timeout == 0 {
		timeout = math.MaxInt64
	}

	select {
	case to <- pkt.Copy():
	case <-time.After(timeout):
		return transport.TimeoutError(timeout)
	}

	return nil
}

// Recv implements transport.Socket.
func (s *PerfSocket) Recv(timeout time.Duration) (transport.Packet, error) {
	s.RLock()
	myChan := s.incomings[s.myAddr]
	s.RUnlock()

	select {
	case <-time.After(timeout):
		return transport.Packet{}, transport.TimeoutError(timeout)
	case pkt := <-myChan:
		return pkt, nil
	}
}

// GetAddress implements transport.Socket.
func (s *PerfSocket) GetAddress() string {
	return s.myAddr
}

// GetIns implements transport.Socket
func (s *PerfSocket) GetIns() []transport.Packet {
	// for performance reasons, we don't try to monitor packets
	panic("perfchannel doesn't implement GetIns - consider using channel transport")
}

// GetOuts implements transport.Socket
func (s *PerfSocket) GetOuts() []transport.Packet {
	// for performance reasons, we don't try to monitor packets
	panic("perfchannel doesn't implement GetOuts - consider using channel transport")
}
