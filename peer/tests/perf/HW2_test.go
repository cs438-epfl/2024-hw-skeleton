//go:build performance
// +build performance

package perf

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport/channel"
)

// This test executes the exact same function as the Benchmark below.
// Its goal is mainly to raise any error that could occur during its execution as the benchmark hides them.
func Test_HW2_UTSRD_Benchmark_Correctness(t *testing.T) {
	runUTSRD(t, 1)
}

// Run BenchmarkUTRSD and compare results to reference assessments
// Run as follow: make test_bench_hw2
func Test_HW2_BenchmarkUTSRD(t *testing.T) {
	// run the benchmark
	res := testing.Benchmark(BenchmarkUTSRD)

	// assess allocation against thresholds, the performance thresholds is the allocation on GitHub
	assessAllocs(t, res, []allocThresholds{
		{"allocs great", 925_000, 260_000_000},
		{"allocs ok", 1_500_000, 400_000_000},
		{"allocs passable", 2_250_000, 593750000},
	})

	// assess execution speed against thresholds, the performance thresholds is the execution speed on GitHub
	assessSpeed(t, res, []speedThresholds{
		{"speed great", 30 * time.Second},
		{"speed ok", 120 * time.Second},
		{"speed passable", 400 * time.Second},
	})
}

// Checks the speed for N nodes to (U)pload, (T)ag, (S)earch, (R)esolve, and
// (D)ownload. Each nodes are connected to each other, and each node gets all
// files from the other nodes.
func BenchmarkUTSRD(b *testing.B) {

	// Disable outputs to not penalize implementations that make use of it
	oldStdout := os.Stdout
	os.Stdout = nil

	defer func() {
		os.Stdout = oldStdout
	}()

	runUTSRD(b, b.N)
}

func runUTSRD(t require.TestingT, rounds int) {
	rand.Seed(1)
	chunkSize := 2048
	maxSize := (chunkSize + len(peer.MetafileSep)) / (64 + len(peer.MetafileSep)) * chunkSize

	transp := channel.NewTransport()

	// Set number of nodes
	numberNodes := 15

	nodes := make([]z.TestNode, numberNodes)
	files := make([][]byte, numberNodes)
	metahashes := make([]string, numberNodes)
	names := make([]string, numberNodes)

	// run as many times as specified by t.N
	for i := 0; i < rounds; i++ {
		// Create N nodes.
		for i := range nodes {
			node := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithChunkSize(2048))
			nodes[i] = node
		}

		// Connect all nodes together.
		for i := range nodes {
			for j := range nodes {
				if i != j {
					nodes[i].AddPeer(nodes[j].GetAddr())
				}
			}
		}

		getFileAndName := func(fsize uint) ([]byte, string) {
			file := make([]byte, fsize)
			_, err := rand.Read(file)
			require.NoError(t, err)

			name := make([]byte, 8)
			_, err = rand.Read(name)
			require.NoError(t, err)

			return file, hex.EncodeToString(name)
		}

		// 1 (UT): (U)pload and (T)ag a file on each node
		for i, node := range nodes {
			file, name := getFileAndName(uint(rand.Intn(maxSize)))

			mh, err := node.Upload(bytes.NewBuffer(file))
			require.NoError(t, err)

			err = node.Tag(name, mh)
			require.NoError(t, err)

			files[i] = file
			metahashes[i] = mh
			names[i] = name
		}

		expandingRing := peer.ExpandingRing{
			Initial: uint(numberNodes),
			Factor:  2,
			Retry:   uint(numberNodes),
			Timeout: time.Second * 10,
		}

		// 2 (SRD): (S)earch, (R)esolve, and (D)ownload. Each node gets all the
		// files.
		for _, node := range nodes {
			for i, name := range names {
				match, err := node.SearchFirst(*regexp.MustCompile(name), expandingRing)
				require.NoError(t, err)
				require.Equal(t, name, match)

				mh := node.Resolve(match)
				require.Equal(t, metahashes[i], mh)

				data, err := node.Download(mh)
				require.NoError(t, err)

				require.Equal(t, files[i], data)
			}
		}
		// Cleanup
		for i := range nodes {
			nodes[i].Stop()
		}
	}
}
