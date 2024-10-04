package disrupted

import (
	"math/rand"
	"time"

	"go.dedis.ch/cs438/transport"
)

// lossSocket represents a Socket dropping packets with a certain probability (drop rate)
type lossSocket struct {
	transport.ClosableSocket
	dropRate float64 //0-1
	randGen  *rand.Rand
}

func (s *lossSocket) Recv(timeout time.Duration) (transport.Packet, error) {
	p, err := s.ClosableSocket.Recv(timeout)
	if err != nil {
		return transport.Packet{}, err
	}
	// if the random float is inferior to the drop rate,
	//we ignore the received packet and get the next one
	val := s.randGen.Float64()
	if val < s.dropRate {
		return s.Recv(timeout)
	}
	return p, nil
}
