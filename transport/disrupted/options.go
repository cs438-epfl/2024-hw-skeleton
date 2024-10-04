package disrupted

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"go.dedis.ch/cs438/transport"
)

// Option represents a wrapper around a Socket
type Option func(transport.ClosableSocket, *rand.Rand) transport.ClosableSocket

func WithLossSocket(dropRate float64) Option {
	return func(rawSocket transport.ClosableSocket, r *rand.Rand) transport.ClosableSocket {
		return &lossSocket{rawSocket, dropRate, r}
	}
}

func withGenericPacketModifier(pm packetModifier, rate float64) Option {
	return func(rawSocket transport.ClosableSocket, r *rand.Rand) transport.ClosableSocket {
		s := insertionSocket{
			ClosableSocket: rawSocket,
			packetModifier: pm,
			insertionRate:  rate,
			randGen:        r,
		}
		s.Start()
		return &s
	}
}

func WithSourceSpoofer(insertionRate float64, spoofedSource string) Option {
	return withGenericPacketModifier(sourceSpoofer(spoofedSource), insertionRate)
}

func WithPacketIDRandomizer(insertionRate float64) Option {
	return withGenericPacketModifier(packetIDRandomizer(), insertionRate)
}

func WithPayloadRandomizer(insertionRate float64, payloadSeed int64) Option {
	return withGenericPacketModifier(payloadRandomizer(), insertionRate)
}

func WithDuplicator(insertionRate float64) Option {
	return withGenericPacketModifier(duplicator(), insertionRate)
}

func WithGenericDelay(delayFunc DelayFunction) Option {
	return func(rawSocket transport.ClosableSocket, r *rand.Rand) transport.ClosableSocket {
		f := delaySocket{rawSocket, delayFunc, sync.WaitGroup{}, nil, r, nil}
		f.Start()
		return &f
	}
}

func WithFixedDelay(value time.Duration) Option {
	return WithGenericDelay(FixedDelay(value))
}

func WithExponentialDelay(mean time.Duration) Option {
	return WithGenericDelay(ExponentialDelay(mean))
}

func WithSineDelay(amp, freq float64) Option {
	return WithGenericDelay(SineDelay(amp, freq))
}

func WithJam(jamTimeout time.Duration, jamBufferSize int) Option {
	if jamTimeout == 0 {
		jamTimeout = math.MaxInt
	}
	return func(rawSocket transport.ClosableSocket, r *rand.Rand) transport.ClosableSocket {
		f := jamSocket{
			ClosableSocket:  rawSocket,
			saturationGroup: sync.WaitGroup{},
			stopGroup:       sync.WaitGroup{},
			jamBufferSize:   jamBufferSize,
			jamTimeout:      jamTimeout,
		}
		f.Start()
		return &f
	}
}

func withTopSocket() Option {
	return func(rawSocket transport.ClosableSocket, r *rand.Rand) transport.ClosableSocket {
		return &topSocket{rawSocket, packets{}}
	}
}
