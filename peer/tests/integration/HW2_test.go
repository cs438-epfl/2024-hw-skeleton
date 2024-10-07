package integration

import (
	"bytes"
	"fmt"
	"math/rand"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/registry/proxy"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/disrupted"
)

// Integration test, with the following topology where * are reference nodes:
// ┌────────────┐
// ▼            │
// C* ────► D   │
// │        │   │
// ▼        ▼   │
// A ◄────► B* ─┘
//
// We split the integration test into multiple, incremental steps, and gradually
// run tests by adding a new step.
//
// The same flow of steps is repeated over a disrupted network, with jammed and/or
// delayed nodes.
func Test_HW2_Integration_Scenario(t *testing.T) {

	scenarios := func(transportA transport.Transport, transportD transport.Transport) func(*testing.T) {
		return func(t *testing.T) {
			setupFunc := setupNetwork(transportA, transportD)
			stages := []stage{
				setupFunc,
				uploadDownload,
				tagAndSearch,
				nodeCDownload,
				nodeDDownload,
				add2Nodes,
				newNodesSearch,
			}

			for i := 1; i < len(stages); i++ {
				maxStage := i
				t.Run(fmt.Sprintf("stage %d", i), func(t *testing.T) {
					t.Parallel()

					s := &state{t: t}
					defer stop(s)

					// iterating over all the stages, from 0 (setup) to maxStage (included)
					for k := 0; k < maxStage+1; k++ {
						stages[k](s)
					}
				})
			}
		}
	}

	t.Run("non-disrupted topology", scenarios(udpFac(), udpFac()))
	t.Run("Jammed node D and node A",
		scenarios(disrupted.NewDisrupted(udpFac(), disrupted.WithJam(time.Second, 8)),
			disrupted.NewDisrupted(udpFac(), disrupted.WithJam(time.Second, 8))))
	t.Run("delayed node A",
		scenarios(disrupted.NewDisrupted(udpFac(), disrupted.WithFixedDelay(500*time.Millisecond)), udpFac()))
}

// state holds all variables needed across stages.
type state struct {
	t          *testing.T
	nodes      map[string]*z.TestNode
	getFile    func(size uint) []byte
	chunkSize  uint
	metahashes map[string]string
	files      map[string][]byte

	referenceTransp transport.Transport
	studentTransp   transport.Transport
	studentOpts     []z.Option
	refOpts         []z.Option
}

// stage represents a new step in the integration test
type stage func(*state) *state

// => Stage 1
//
// setupNetwork generates a setup function given the transport layers of nodes A and D
func setupNetwork(transportA transport.Transport, transportD transport.Transport) stage {
	return func(s *state) *state {
		s.t.Log("~~ stage 1 <> setup ~~")

		r := rand.New(rand.NewSource(1))

		referenceTransp := proxyFac()

		chunkSize := uint(2048)

		studentOpts := []z.Option{
			z.WithChunkSize(chunkSize),
			z.WithHeartbeat(time.Second * 200),
			z.WithAntiEntropy(time.Second * 5),
			z.WithAckTimeout(time.Second * 10),
		}
		refOpts := append(studentOpts, z.WithMessageRegistry(proxy.NewRegistry()))

		// first and second transport layers of the array are set to nodes A and D respectively
		nodeA := z.NewTestNode(s.t, studentFac, transportA, "127.0.0.1:0", studentOpts...)
		nodeB := z.NewTestNode(s.t, referenceFac, referenceTransp, "127.0.0.1:0", refOpts...)
		nodeC := z.NewTestNode(s.t, referenceFac, referenceTransp, "127.0.0.1:0", refOpts...)
		nodeD := z.NewTestNode(s.t, studentFac, transportD, "127.0.0.1:0", studentOpts...)

		nodeA.AddPeer(nodeB.GetAddr())
		nodeB.AddPeer(nodeA.GetAddr())
		nodeB.AddPeer(nodeC.GetAddr())
		nodeC.AddPeer(nodeA.GetAddr())
		nodeC.AddPeer(nodeD.GetAddr())
		nodeD.AddPeer(nodeB.GetAddr())

		s.nodes = map[string]*z.TestNode{
			"nodeA": &nodeA,
			"nodeB": &nodeB,
			"nodeC": &nodeC,
			"nodeD": &nodeD,
		}

		s.getFile = func(size uint) []byte {
			file := make([]byte, size)
			_, err := r.Read(file)
			require.NoError(s.t, err)
			return file
		}

		s.chunkSize = uint(2048)

		s.files = make(map[string][]byte)
		s.metahashes = make(map[string]string)

		// the student transport layer is set to UDP, and is used for nodes E and F.
		s.studentTransp = udpFac()
		s.referenceTransp = referenceTransp
		s.studentOpts = studentOpts
		s.refOpts = refOpts

		time.Sleep(time.Second * 10)

		return s
	}
}

// => Stage 2
//
// Upload a file on B and downloads it.
func uploadDownload(s *state) *state {
	s.t.Log("~~ stage 2 <> upload and download ~~")

	// > If I upload a file on NodeB I should be able to download it from NodeB

	fileB := s.getFile(s.chunkSize*3 + 123)
	mhB, err := s.nodes["nodeB"].Upload(bytes.NewBuffer(fileB))
	require.NoError(s.t, err)

	res, err := s.nodes["nodeB"].Download(mhB)
	require.NoError(s.t, err)
	require.Equal(s.t, fileB, res)

	s.files["fileB"] = fileB
	s.metahashes["mhB"] = mhB

	return s
}

// => Stage 3
//
// Tag B's file and retrieve it from node A.
func tagAndSearch(s *state) *state {
	s.t.Log("~~ stage 3 <> tag and search ~~")

	// Let's tag file on NodeB
	err := s.nodes["nodeB"].Tag("fileB", s.metahashes["mhB"])
	require.NoError(s.t, err)

	// > NodeA should be able to index file from B
	names, err := s.nodes["nodeA"].SearchAll(*regexp.MustCompile("file.*"), 3, time.Second*3)
	require.NoError(s.t, err)
	require.Len(s.t, names, 1)
	require.Equal(s.t, "fileB", names[0])

	// > NodeA should have added "fileB" in its naming storage
	mhB2 := s.nodes["nodeA"].Resolve(names[0])
	require.Equal(s.t, s.metahashes["mhB"], mhB2)

	// > I should be able to download fileB from nodeA
	res, err := s.nodes["nodeA"].Download(s.metahashes["mhB"])
	require.NoError(s.t, err)
	require.Equal(s.t, s.files["fileB"], res)

	return s
}

// => Stage 4
//
// Index and download B's file from C.
func nodeCDownload(s *state) *state {
	s.t.Log("~~ stage 4 <> nodeC download ~~")

	// > NodeC should be able to index fileB from A
	names, err := s.nodes["nodeC"].SearchAll(*regexp.MustCompile("fileB"), 3, time.Second*4)
	require.NoError(s.t, err)
	require.Len(s.t, names, 1)
	require.Equal(s.t, "fileB", names[0])

	// > NodeC should have added "fileB" in its naming storage
	mhB2 := s.nodes["nodeC"].Resolve(names[0])
	require.Equal(s.t, s.metahashes["mhB"], mhB2)

	// > I should be able to download fileB from nodeC
	res, err := s.nodes["nodeC"].Download(s.metahashes["mhB"])
	require.NoError(s.t, err)
	require.Equal(s.t, s.files["fileB"], res)

	return s
}

// => Stage 5
//
// // Index and download B's file from D.
func nodeDDownload(s *state) *state {
	s.t.Log("~~ stage 5 <> nodeD download ~~")

	// Add a file on node D
	fileD := s.getFile(s.chunkSize*3 - 123)
	mhD, err := s.nodes["nodeD"].Upload(bytes.NewBuffer(fileD))
	require.NoError(s.t, err)
	err = s.nodes["nodeD"].Tag("fileD", mhD)
	require.NoError(s.t, err)
	// > NodeA should be able to search for "fileD" using the expanding scheme
	conf := peer.ExpandingRing{
		Initial: 1,
		Factor:  2,
		Retry:   5,
		Timeout: time.Second * 3,
	}
	name, err := s.nodes["nodeA"].SearchFirst(*regexp.MustCompile("fileD"), conf)
	require.NoError(s.t, err)
	require.Equal(s.t, "fileD", name)

	// > NodeA should be able to download fileD
	mhD2 := s.nodes["nodeA"].Resolve(name)
	require.Equal(s.t, mhD, mhD2)

	res, err := s.nodes["nodeA"].Download(mhD2)
	require.NoError(s.t, err)
	require.Equal(s.t, fileD, res)

	return s
}

// => Stage 6
//
// Add two new nodes.
func add2Nodes(s *state) *state {
	s.t.Log("~~ stage 6 <> add 2 nodes ~~")

	// Let's add new nodes and see if they can index the files and download them

	//             ┌───────────┐
	//             ▼           │
	// F* ──► E ─► C ────► D   │
	//             │       │   │
	//             ▼       ▼   │
	//             A ◄───► B ◄─┘

	nodeE := z.NewTestNode(s.t, studentFac, s.studentTransp, "127.0.0.1:0", s.studentOpts...)
	nodeF := z.NewTestNode(s.t, referenceFac, s.referenceTransp, "127.0.0.1:0", s.refOpts...)

	s.nodes["nodeE"] = &nodeE
	s.nodes["nodeF"] = &nodeF

	nodeE.AddPeer(s.nodes["nodeC"].GetAddr())
	nodeF.AddPeer(s.nodes["nodeE"].GetAddr())

	time.Sleep(time.Second * 10)

	return s
}

// => Stage 7
//
// Search files from the new nodes.
func newNodesSearch(s *state) *state {
	s.t.Log("~~ stage 7 <> new nodes search ~~")

	// > NodeF should be able to index all files (2)

	names, err := s.nodes["nodeF"].SearchAll(*regexp.MustCompile(".*"), 8, time.Second*4)
	require.NoError(s.t, err)
	require.Len(s.t, names, 2)
	require.Contains(s.t, names, "fileB")
	require.Contains(s.t, names, "fileD")

	// > NodeE should be able to search for fileB
	conf := peer.ExpandingRing{
		Initial: 1,
		Factor:  2,
		Retry:   4,
		Timeout: time.Second * 3,
	}
	name, err := s.nodes["nodeE"].SearchFirst(*regexp.MustCompile("fileB"), conf)
	require.NoError(s.t, err)
	require.Equal(s.t, "fileB", name)

	return s
}

// stop stops all nodes.
func stop(s *state) *state {
	s.t.Log("~~ stop ~~")

	for _, node := range s.nodes {
		n, ok := node.Peer.(z.Terminable)
		if ok {
			err := n.Terminate()
			require.NoError(s.t, err)
		} else {
			err := node.Stop()
			require.NoError(s.t, err)
		}
	}

	return s
}
