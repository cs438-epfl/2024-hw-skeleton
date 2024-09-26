package disrupted

import (
	"time"

	"go.dedis.ch/cs438/transport"
)

// topSocket is a virtual, pass-through internal socket overriding GetIns
// It is meant to be used atop other virtual, filtering sockets.
type topSocket struct {
	transport.ClosableSocket
	ins packets
}

// GetIns implements transport.Socket
func (s *topSocket) GetIns() []transport.Packet {
	return s.ins.getAll()
}

func (s *topSocket) Recv(timeout time.Duration) (transport.Packet, error) {
	p, err := s.ClosableSocket.Recv(timeout)
	if err != nil {
		return transport.Packet{}, err
	}

	s.ins.add(p)
	return p, nil
}
