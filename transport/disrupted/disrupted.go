package disrupted

import (
	"math/rand"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
	"golang.org/x/xerrors"
)

// Constants used by DisruptedSockets

const chanSize = 1024                        // Size of buffered channel to use, when relevant
const poolSize = 100                         // (max) Size of worker pool to use, when relevant
const packetTimeout = 500 * time.Millisecond //

// Transport implements a transport layer wrapper simulating network glitches
//
// - implements transport.Transport
type Transport struct {
	transport.Transport
	options []Option
	randGen *rand.Rand
}

// NewDisrupted returns a new disrupted transport implementation.
func NewDisrupted(t transport.Transport, o ...Option) *Transport {
	return &Transport{t, o, rand.New(rand.NewSource(0))}
}

// SetRandomGenSeed utility method for changing the seed of the random generator
func (t *Transport) SetRandomGenSeed(seed int64) {
	t.randGen.Seed(seed)
}

// CreateSocket implements transport.Transport
func (t *Transport) CreateSocket(address string) (transport.ClosableSocket, error) {
	s, err := t.Transport.CreateSocket(address)
	if err != nil {
		return nil, xerrors.Errorf("failed to create underlying socket: %v", err)
	}
	for _, opt := range t.options {
		s = opt(s, t.randGen)
	}
	s = withTopSocket()(s, t.randGen)
	return s, nil
}

type packets struct {
	sync.Mutex
	data []transport.Packet
}

func (p *packets) add(pkt transport.Packet) {
	p.Lock()
	defer p.Unlock()

	p.data = append(p.data, pkt.Copy())
}

func (p *packets) getAll() []transport.Packet {
	p.Lock()
	defer p.Unlock()

	res := make([]transport.Packet, len(p.data))

	for i, pkt := range p.data {
		res[i] = pkt.Copy()
	}

	return res
}
