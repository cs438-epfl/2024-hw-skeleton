//go:build performance
// +build performance

package perf

import (
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"

	"go.dedis.ch/cs438/registry/standard"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

// This test executes the exact same function as the Benchmark below.
// Its goal is mainly to raise any error that could occur during its execution as the benchmark hides them.
func Test_HW1_SpamNode_Benchmark_Correctness(t *testing.T) {
	spamNode(t, 1)
}

// Run BenchmarkUTRSD and compare results to reference assessments
// Run as follow: make test_bench_hw1
func Test_HW1_BenchmarkSpamNode(t *testing.T) {
	// run the benchmark
	res := testing.Benchmark(BenchmarkSpamNode)
	// assess allocation against thresholds, the performance thresholds is the allocation on GitHub
	assessAllocs(t, res, []allocThresholds{
		{"allocs great", 109_000, 515_000_000},
		{"allocs ok", 182_000, 860_000_000},
		{"allocs passable", 280_000, 1_290_000_000},
	})

	// assess execution speed against thresholds, the performance thresholds is the execution speed on GitHub
	assessSpeed(t, res, []speedThresholds{
		{"speed great", 3 * time.Second},
		{"speed ok", 20 * time.Second},
		{"speed passable", 80 * time.Second},
	})
}

// Flood a node with n (-benchtime argument) messages and wait on rumors to be processed.
// Rumors are sent with expected sequences, so they should be processed.
func BenchmarkSpamNode(b *testing.B) {
	// Disable outputs to not penalize implementations that make use of it
	oldStdout := os.Stdout
	os.Stdout = nil

	defer func() {
		os.Stdout = oldStdout
	}()

	spamNode(b, b.N)
}

func spamNode(t require.TestingT, rounds int) {
	transp := channelFac()

	fake := z.NewFakeMessage(t)

	// set number of messages to be sent (per benchmark iteration)
	numberMessages := 1000

	notifications := make(chan struct{}, rounds*numberMessages)

	handler := func(types.Message, transport.Packet) error {
		notifications <- struct{}{}
		return nil
	}

	sender, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	receiver := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithMessage(fake, handler),
		z.WithContinueMongering(0))

	src := sender.GetAddress()
	dst := receiver.GetAddr()

	currentRumorSeq := 0
	ackID := make([]byte, 12)

	// run as many times as specified by rounds
	for i := 0; i < rounds; i++ {
		// send numberMessages messages (hard-coded)
		for j := 0; j < numberMessages; j++ {
			// flip a coin to send either a rumor or an ack message
			coin := rand.Float64() > 0.5

			if coin {
				currentRumorSeq++

				err = sendRumor(fake, uint(currentRumorSeq), src, dst, sender)
				require.NoError(t, err)
			} else {
				_, err = rand.Read(ackID)
				require.NoError(t, err)

				err = sendAck(string(ackID), src, dst, sender)
				require.NoError(t, err)
			}
		}
	}

	// > wait on all the rumors to be processed

	for i := 0; i < currentRumorSeq; i++ {
		select {
		case <-notifications:
		case <-time.After(time.Second):
			t.Errorf("notification not received in time")
		}
	}

	// cleanup
	receiver.Stop()
	sender.Close()
}

func sendRumor(msg types.Message, seq uint, src, dst string, sender transport.Socket) error {

	embedded, err := defaultRegistry.MarshalMessage(msg)
	if err != nil {
		return xerrors.Errorf("failed to marshal msg: %v", err)
	}

	rumor := types.Rumor{
		Origin:   src,
		Sequence: seq,
		Msg:      &embedded,
	}

	rumorsMessage := types.RumorsMessage{
		Rumors: []types.Rumor{rumor},
	}

	transpMsg, err := defaultRegistry.MarshalMessage(rumorsMessage)
	if err != nil {
		return xerrors.Errorf("failed to marshal transp msg: %v", err)
	}

	header := transport.NewHeader(src, src, dst)

	pkt := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = sender.Send(dst, pkt, 0)
	if err != nil {
		return xerrors.Errorf("failed to send message: %v", err)
	}

	return nil
}

func sendAck(ackedPacketID string, src, dst string, sender transport.Socket) error {
	ack := types.AckMessage{
		AckedPacketID: ackedPacketID,
	}

	registry := standard.NewRegistry()

	msg, err := registry.MarshalMessage(ack)
	if err != nil {
		return xerrors.Errorf("failed to marshal message: %v", err)
	}

	header := transport.NewHeader(src, src, dst)

	pkt := transport.Packet{
		Header: &header,
		Msg:    &msg,
	}

	err = sender.Send(dst, pkt, 0)
	if err != nil {
		return xerrors.Errorf("failed to send message: %v", err)
	}

	return nil
}
