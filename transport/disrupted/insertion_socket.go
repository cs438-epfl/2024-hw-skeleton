package disrupted

import (
	"encoding/json"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/rs/xid"
	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// insertionSocket handles duplication and injection.
// It uses the packetModifier type to provide duplication, source address spoofing, randomized packet ID and
// randomized payload.
type insertionSocket struct {
	transport.ClosableSocket
	insertionRate  float64 // 0-1
	randGen        *rand.Rand
	packetModifier packetModifier
	wg             sync.WaitGroup
	in             chan transport.Packet
	out            chan transport.Packet
	stop           chan struct{}
}

// Generic packetModifier type: using a packet as seed,
// can insert any number of other (possibly modified) packets into queue
type packetModifier func(p transport.Packet, out chan transport.Packet, r *rand.Rand)

func (s *insertionSocket) Start() { // non-blocking
	s.in = make(chan transport.Packet, chanSize)
	s.out = make(chan transport.Packet)
	s.stop = make(chan struct{})
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		for pkt := range s.in {
			s.out <- pkt
			if s.randGen.Float64() <= s.insertionRate {
				s.packetModifier(pkt, s.out, s.randGen)
			}
		}
		close(s.out)
	}()
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.stop:
				close(s.in)
				return
			default:
				p, err := s.ClosableSocket.Recv(time.Second)
				if err == nil {
					s.in <- p
				}
			}
		}
	}()
}

func (s *insertionSocket) Close() error {
	close(s.stop)
	s.wg.Wait()
	return nil
}

func (s *insertionSocket) Recv(timeout time.Duration) (transport.Packet, error) {
	select {
	case p, ok := <-s.out:
		if !ok {
			return transport.Packet{},
				xerrors.Errorf("Error while trying to receive a packet in the insertion socket: channel closed")
		}
		return p, nil
	case <-time.After(timeout):
		return transport.Packet{}, transport.TimeoutError(timeout)
	}
}

// Duplicate
func duplicator() packetModifier {
	return func(packet transport.Packet, out chan transport.Packet, r *rand.Rand) {
		out <- packet.Copy()
	}
}

// Duplicate but change the source string
func sourceSpoofer(spoofedSource string) packetModifier {
	return func(packet transport.Packet, out chan transport.Packet, r *rand.Rand) {
		h := packet.Header.Copy()
		h.Source = spoofedSource
		msg := packet.Msg.Copy()
		out <- transport.Packet{
			Header: &h,
			Msg:    &msg,
		}
	}
}

// Duplicate but change the packetID
func packetIDRandomizer() packetModifier {
	return func(packet transport.Packet, out chan transport.Packet, r *rand.Rand) {
		h := packet.Header.Copy()
		h.PacketID = xid.New().String()
		msg := packet.Msg.Copy()
		out <- transport.Packet{
			Header: &h,
			Msg:    &msg,
		}
	}
}

// Duplicate but change the payload
func payloadRandomizer() packetModifier {
	return func(packet transport.Packet, out chan transport.Packet, r *rand.Rand) {
		randomPayload := r.Intn(math.MaxInt32)
		b, _ := json.Marshal(randomPayload)
		h := packet.Header.Copy()
		out <- transport.Packet{
			Header: &h,
			Msg: &transport.Message{
				Type:    packet.Msg.Type,
				Payload: b,
			},
		}
	}
}
