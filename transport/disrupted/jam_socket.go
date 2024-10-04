package disrupted

import (
	"math"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// jamSocket is a socket buffering messages until threshold or timeout,
// then transmits all buffered messages (possibly reordering)
type jamSocket struct {
	transport.ClosableSocket
	saturationGroup sync.WaitGroup
	stopGroup       sync.WaitGroup
	in              chan transport.Packet
	out             chan transport.Packet
	jamBufferSize   int
	jamTimeout      time.Duration
	stop            chan struct{}
}

func (s *jamSocket) Start() { // non-blocking
	s.out = make(chan transport.Packet, chanSize)
	s.in = make(chan transport.Packet, s.jamBufferSize)
	s.stop = make(chan struct{})
	s.saturationGroup = sync.WaitGroup{}
	s.stopGroup = sync.WaitGroup{}

	// Avoid goroutine explosion: limit to poolSize the number of workers
	if s.jamBufferSize > poolSize {
		s.jamBufferSize = poolSize
	}
	go s.batcher()
}

// epoch-based batcher, launching listeners and waiting for them to terminate
func (s *jamSocket) batcher() {
	s.stopGroup.Add(1)
	defer s.stopGroup.Done()
	for {
		s.saturationGroup.Add(s.jamBufferSize)
		for i := 0; i < s.jamBufferSize; i++ {
			go s.listenerWorker()
		}
		s.saturationGroup.Wait() // eventually terminates
		for i := 0; i < s.jamBufferSize; i++ {
			select {
			case pkt := <-s.in:
				s.out <- pkt
			default:
			}
		}
		select {
		case <-s.stop:
			close(s.in)
			return
		default:
		}
	}
}

// Worker function, always eventually terminates: either
//   - receives a message
//   - times out on jamTimeout
//   - exits on stop channel
func (s *jamSocket) listenerWorker() {
	defer s.saturationGroup.Done()
	timeoutChannel := time.After(s.jamTimeout)
	for {
		select {
		case <-s.stop:
			return
		case <-timeoutChannel:
			return
		default:
			p, err := s.ClosableSocket.Recv(packetTimeout)
			if err == nil {
				s.in <- p
				return
			}
		}
	}
}
func (s *jamSocket) Recv(timeout time.Duration) (p transport.Packet, err error) {
	if timeout == 0 {
		timeout = math.MaxInt
	}
	select {
	case p, ok := <-s.out:
		if !ok {
			return transport.Packet{},
				xerrors.Errorf("Error while trying to receive a packet in the jam socket: channel closed")
		}
		return p, nil
	case <-time.After(timeout):
		return transport.Packet{}, transport.TimeoutError(timeout)
	}
}

func (s *jamSocket) Close() error {
	close(s.stop)
	s.stopGroup.Wait()
	close(s.out)
	return nil
}
