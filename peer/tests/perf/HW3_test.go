//go:build performance
// +build performance

package perf

import (
	"bytes"
	"crypto/sha256"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

// This test executes the exact same function as the BenchmarkTLC below.
// Its goal is mainly to raise any error that could occur during its execution as the benchmark hides them.
func Test_HW3_TLC_Benchmark_Correctness(t *testing.T) {
	runTLC(t, 10, 1)
}

// This test executes the exact same function as the Benchmark Consensus below.
// Its goal is mainly to raise any error that could occur during its execution as the benchmark hides them.
func Test_HW3_Consensus_Benchmark_Correctness(t *testing.T) {
	runConsensus(t, 1)
}

// Run BenchmarkTLC and compare results to reference assessments
// Run as follow: make test_bench_hw3_tlc
func Test_HW3_BenchmarkTLC(t *testing.T) {
	// run the benchmark
	res := testing.Benchmark(BenchmarkTLC)

	// assess allocation against thresholds, the performance thresholds is the allocation on GitHub
	assessAllocs(t, res, []allocThresholds{
		{"allocs great", 123_000, 12_500_000},
		{"allocs ok", 165_000, 15_150_000},
		{"allocs passable", 235_000, 20_188_000},
	})

	// assess execution speed against thresholds, the performance thresholds is the execution speed on GitHub
	assessSpeed(t, res, []speedThresholds{
		{"speed great", 60 * time.Millisecond},
		{"speed ok", 200 * time.Millisecond},
		{"speed passable", 800 * time.Millisecond},
	})
}

// Run BenchmarkConsensus and compare results to reference assessments
// Run as follow: make test_bench_hw3_consensus
func Test_HW3_BenchmarkConsensus(t *testing.T) {
	// run the benchmark
	res := testing.Benchmark(BenchmarkConsensus)

	// assess allocation against thresholds, the performance thresholds is the allocation on GitHub
	assessAllocs(t, res, []allocThresholds{
		{"allocs great", 1_315_000, 79_100_000},
		{"allocs ok", 2_061_000, 124_800_000},
		{"allocs passable", 2_809_000, 250_000_000},
	})

	// assess execution speed against thresholds, the performance thresholds is the execution speed on GitHub
	assessSpeed(t, res, []speedThresholds{
		{"speed great", 5 * time.Second},
		{"speed ok", 15 * time.Second},
		{"speed passable", 60 * time.Second},
	})
}

// Spam a node with TLC messages and check that it correctly processed them. We
// are going to randomly send messages filling the interval [0,N), and [N+1,2N].
// In this case the node should process the [0, N) TLC messages and create N
// blockchain blocks.
func BenchmarkTLC(b *testing.B) {

	// Disable outputs to not penalize implementations that make use of it
	oldStdout := os.Stdout
	os.Stdout = nil
	defer func() {
		os.Stdout = oldStdout
	}()

	runTLC(b, 10000, b.N)
}

func runTLC(t require.TestingT, nodeCount, rounds int) {
	// run as many times as specified by rounds
	for i := 0; i < rounds; i++ {
		rand.Seed(1)

		// not enough to create the intervals
		if nodeCount == 1 {
			panic("this test is meaningless for nodeCount=1")
		}

		blockchain := getBlockchain(nodeCount * 2)
		expectedLastHash := blockchain[nodeCount-1].Hash
		// create a hole
		blockchain = append(blockchain[:nodeCount], blockchain[nodeCount+1:]...)

		rand.Shuffle(len(blockchain), func(i, j int) {
			blockchain[i], blockchain[j] = blockchain[j], blockchain[i]
		})

		transp := channelFac()

		threshold := func(uint) int {
			return 1
		}

		node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1),
			z.WithPaxosThreshold(threshold))

		sender, err := z.NewSenderSocket(transp, "127.0.0.1:0")
		require.NoError(t, err)

		node1.AddPeer(sender.GetAddress())

		for _, block := range blockchain {
			tlc := types.TLCMessage{
				Step:  block.Index,
				Block: block,
			}

			transpMsg, err := node1.GetRegistry().MarshalMessage(&tlc)
			require.NoError(t, err)

			header := transport.NewHeader(sender.GetAddress(), sender.GetAddress(), node1.GetAddr())

			packet := transport.Packet{
				Header: &header,
				Msg:    &transpMsg,
			}

			err = sender.Send(node1.GetAddr(), packet, 0)
			require.NoError(t, err)
		}

		store := node1.GetStorage().GetBlockchainStore()

		// wait proportionally to the number of messages
		for i := 0; i < nodeCount*100; i++ {
			if bytes.Equal(expectedLastHash, store.Get(storage.LastBlockKey)) {
				return
			}
			time.Sleep(time.Microsecond)
		}

		t.Errorf("invalid last hash: %x != %x", expectedLastHash, store.Get(storage.LastBlockKey))

		// cleanup
		sender.Close()
		node1.Stop()
	}
}

// Makes each node tag something, and wait until all proposals are accepted.
func BenchmarkConsensus(b *testing.B) {

	// Disable outputs to not penalize implementations that make use of it
	oldStdout := os.Stdout
	os.Stdout = nil

	defer func() {
		os.Stdout = oldStdout
	}()

	runConsensus(b, b.N)
}

func runConsensus(t require.TestingT, rounds int) {
	transp := channelFac()

	// Set number of nodes
	numberNodes := 5

	nodes := make([]z.TestNode, numberNodes)
	wait := sync.WaitGroup{}

	// run as many times as specified by rounds
	for i := 0; i < rounds; i++ {
		for i := range nodes {
			node := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
				z.WithTotalPeers(uint(numberNodes)), z.WithPaxosID(uint(i+1)))
			nodes[i] = node
		}

		for _, n1 := range nodes {
			for _, n2 := range nodes {
				n1.AddPeer(n2.GetAddr())
			}
		}

		wait.Add(numberNodes)

		for _, node := range nodes {
			go func(n z.TestNode) {
				defer wait.Done()

				// this is a blocking call that returns once the proposal is
				// accepted.
				err := n.Tag(xid.New().String(), "1")
				require.NoError(t, err)

				time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))

			}(node)
		}

		wait.Wait()
		// cleanup
		for i := range nodes {
			nodes[i].Stop()
		}
	}
}

// -----------------------------------------------------------------------------
// Utility functions

// computeBlockHash computes the hash of a block
func computeBlockHash(block types.BlockchainBlock) []byte {
	h := sha256.New()

	h.Write([]byte(strconv.Itoa(int(block.Index))))
	h.Write([]byte(block.Value.Filename))
	h.Write([]byte(block.Value.Metahash))
	h.Write(block.PrevHash)

	return h.Sum(nil)
}

// getBlockchain returns a valid blockchain with n blocks.
func getBlockchain(n int) []types.BlockchainBlock {
	blockchain := make([]types.BlockchainBlock, n)

	prevhash := make([]byte, 32)

	for i := range blockchain {
		block := types.BlockchainBlock{
			Index: uint(i),
			Value: types.PaxosValue{
				Filename: xid.New().String(),
				Metahash: xid.New().String(),
			},
			PrevHash: append([]byte{}, prevhash...),
		}
		block.Hash = computeBlockHash(block)
		prevhash = block.Hash

		blockchain[i] = block
	}

	return blockchain
}
