package disrupted

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// delaySocket handles delaying.
// It uses the delayFunction type to provide parametrized pseudo-random delays
type delaySocket struct {
	transport.ClosableSocket
	delayFunc      DelayFunction
	wg             sync.WaitGroup
	delayedPackets chan transport.Packet
	randGen        *rand.Rand
	stop           chan struct{}
}

// Generic Delay Function type
type DelayFunction func(t time.Time, r *rand.Rand) time.Duration

func (s *delaySocket) Start() { // non-blocking
	s.delayedPackets = make(chan transport.Packet, chanSize)
	s.stop = make(chan struct{})
	s.wg = sync.WaitGroup{}
	s.wg.Add(poolSize)
	listener := func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.stop:
				return
			default:
				p, err := s.ClosableSocket.Recv(packetTimeout)
				if err == nil {
					time.Sleep(s.delayFunc(time.Now(), s.randGen))
					s.delayedPackets <- p
				}
			}
		}
	}
	for i := 0; i < poolSize; i++ {
		go listener()
	}
}

func (s *delaySocket) Recv(timeout time.Duration) (p transport.Packet, err error) {
	if timeout == 0 {
		timeout = math.MaxInt
	}
	select {
	case p, ok := <-s.delayedPackets:
		if !ok {
			return transport.Packet{},
				xerrors.Errorf("Error while trying to receive a packet in the delay socket: channel closed")
		}
		return p, nil
	case <-time.After(timeout):
		return transport.Packet{}, transport.TimeoutError(timeout)
	}
}

func (s *delaySocket) Close() error {
	close(s.stop)
	s.wg.Wait()
	close(s.delayedPackets)
	return nil
}

// FixedDelay returns, given a duration, a constant delay function
func FixedDelay(value time.Duration) DelayFunction {
	return func(t time.Time, r *rand.Rand) time.Duration { return value }
}

// ExponentialDelay computes a delay from an exponential distribution,
// a typical simulation of 'realistic' network conditions.
func ExponentialDelay(mean time.Duration) DelayFunction {
	return func(t time.Time, r *rand.Rand) time.Duration {
		return time.Duration(int(r.ExpFloat64()*float64(mean.Microseconds()))) * time.Microsecond
	}
}

// SineDelay provides a way to create sinusoidal delays, simulating traffic jams on networks
func SineDelay(amp, freq float64) DelayFunction {
	return func(t time.Time, r *rand.Rand) time.Duration {
		// we take the absolute value of the sinus as negative durations are considered to be 0s
		return time.Duration(amp * math.Abs(math.Sin(2*math.Pi*freq*float64(t.Second()))))
	}
}
