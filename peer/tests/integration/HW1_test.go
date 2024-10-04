package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"go.dedis.ch/cs438/registry/proxy"
	"go.dedis.ch/cs438/transport/disrupted"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/cs438/internal/graph"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
)

var antiEntropy = time.Second * 10
var ackTimeout = time.Second * 10
var waitPerNode = time.Second * 15

var nodesTimeout = 30 * time.Second

func WaitOrTimeout(t *testing.T, wg *sync.WaitGroup, timeout time.Duration, msg string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done: // wait completed
	case <-time.After(timeout):
		t.Error(msg)
	}
}

func stopAllNodesWithin(t *testing.T, nodes []z.TestNode, delay time.Duration) {
	wait := sync.WaitGroup{}
	wait.Add(len(nodes))

	for i := range nodes {
		go func(node z.TestNode) {
			defer wait.Done()
			node.Stop()
		}(nodes[i])
	}
	t.Log("stopping nodes...")

	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(delay):
			t.Error("timeout on node stop")
		}
	}()
	wait.Wait()
	close(done)
}

func terminateNodes(t *testing.T, nodes []z.TestNode) {
	for i := range nodes {
		n, ok := nodes[i].Peer.(z.Terminable)
		if ok {
			err := n.Terminate()
			require.NoError(t, err)
		}
	}
}

func broadcastChat(t *testing.T, node z.TestNode, wait *sync.WaitGroup, chatMsg string) {
	defer wait.Done()

	chat := types.ChatMessage{
		Message: fmt.Sprintf(chatMsg, node.GetAddr()),
	}
	data, err := json.Marshal(&chat)
	require.NoError(t, err)

	msg := transport.Message{
		Type:    chat.Name(),
		Payload: data,
	}

	err = node.Broadcast(msg)
	require.NoError(t, err)
	time.Sleep(waitPerNode) // Wait for message to be passed around
}

func fetchMessages(nodes []z.TestNode, msgs [][]*types.ChatMessage, out io.Writer, wait *sync.WaitGroup, startT time.Time) {
	l := sync.RWMutex{}
	fmt.Fprintf(out, "stats on %s\n", runtime.GOOS)
	fmt.Fprintf(out, "address, average received [pkt/s], average sent [pkt/s]\n")
	runner := func(i int, node z.TestNode) {
		defer wait.Done()
		msgs[i] = node.GetChatMsgs()

		totSent := len(node.GetOuts())
		totReceived := len(node.GetIns())

		// print stats of throughput for each node
		l.Lock()
		fmt.Fprintf(out, "%s, %f, %f\n", node.GetAddr(),
			float64(totReceived)/float64(time.Since(startT)),
			float64(totSent)/float64(time.Since(startT)))
		l.Unlock()
	}
	for i, node := range nodes {
		go runner(i, node)
	}
}

// 1-14
//
// Make every node send a broadcast message to every other nodes, with a random
// topology.
func Test_HW1_Integration_Broadcast_Random(t *testing.T) {
	getTestRandom := func(commonLayer transport.Transport, disruptedLayer *disrupted.Transport) func(*testing.T) {
		return func(t *testing.T) {

			// We skip any variation of this test for Windows OS due to the underlying network stack
			skipIfWIndows(t)
			if disruptedLayer != nil {
				disruptedLayer.SetRandomGenSeed(1)
			}
			nReference := 10
			nStudent := 10

			// the "commonLayer" is the transport layer of all (except one, optionally) student nodes
			studentLayer := commonLayer
			// the "referenceLayer" is the transport layer of all reference nodes
			referenceLayer := proxyFac()

			nodes := make([]z.TestNode, nReference+nStudent)

			for i := 0; i < nReference; i++ {
				nodes[i] = z.NewTestNode(t, referenceFac, referenceLayer, "127.0.0.1:0",
					z.WithMessageRegistry(proxy.NewRegistry()),
					z.WithAntiEntropy(antiEntropy),
					// since everyone is sending a rumor, there is no need to have route
					// rumors
					z.WithHeartbeat(0),
					z.WithAckTimeout(ackTimeout))

			}

			// we stop at the current index, i.e. at the end of the reference nodes.
			// we then check whether a transport layer way provided for a single node (i.e. a disrupted layer)
			// if it is the case, we set this transport layer to one of the student-transport nodes.
			current := nReference
			if disruptedLayer != nil {
				nodes[current] = z.NewTestNode(t, studentFac, disruptedLayer, "127.0.0.1:0",
					z.WithAntiEntropy(antiEntropy),
					z.WithHeartbeat(0),
					z.WithAckTimeout(ackTimeout))
				current++
			}

			for i := current; i < nReference+nStudent; i++ {
				nodes[i] = z.NewTestNode(t, studentFac, studentLayer, "127.0.0.1:0",
					z.WithAntiEntropy(antiEntropy),
					// since everyone is sending a rumor, there is no need to have route
					// rumors
					z.WithHeartbeat(0),
					z.WithAckTimeout(ackTimeout))
			}

			defer terminateNodes(t, nodes)
			defer stopAllNodesWithin(t, nodes, nodesTimeout)

			// generate and apply a random topology
			// out, err := os.Create("topology.dot")
			// require.NoError(t, err)
			graph.NewGraph(0.2).Generate(io.Discard, nodes)

			// > make each node broadcast a rumor, each node should eventually get
			// rumors from all the other nodes.

			wait := sync.WaitGroup{}
			wait.Add(len(nodes))

			startT := time.Now()

			for i := range nodes {
				go broadcastChat(t, nodes[i], &wait, "hi from %s")
			}

			// Wait for all senders to terminate (or shortly timeout, broadcast should not be blocking)
			WaitOrTimeout(t, &wait, nodesTimeout, "timeout on node broadcast")

			// > check that each node got all the chat messages
			wait.Add(len(nodes))
			nodesChatMsgs := make([][]*types.ChatMessage, len(nodes))
			out := new(strings.Builder)

			// fetching messages can take a bit of time with the proxy, which is why
			// we do it concurrently.
			go fetchMessages(nodes, nodesChatMsgs, out, &wait, startT)
			WaitOrTimeout(t, &wait, nodesTimeout, "timeout on message fetch") // no slow operations there

			// Display stats if a test fails
			t.Log(out.String())

			// > each node should get the same messages as the first node. We sort the
			// messages to compare them.

			expected := nodesChatMsgs[0]
			sort.Sort(types.ChatByMessage(expected))

			// t.Logf("expected chat messages: %v", expected)
			require.Len(t, expected, len(nodes))

			for i := 1; i < len(nodesChatMsgs); i++ {
				compare := nodesChatMsgs[i]
				sort.Sort(types.ChatByMessage(compare))
				require.Equal(t, len(expected), len(compare))
				require.Equal(t, expected, compare)
			}

			// > every node should have an entry to every other node in their routing
			// tables.

			for _, node := range nodes {
				table := node.GetRoutingTable()
				require.Len(t, table, len(nodes), node.GetAddr())

				for _, otherNode := range nodes {
					_, ok := table[otherNode.GetAddr()]
					require.True(t, ok)
				}

				// uncomment the following to generate the routing table graphs
				// out, err := os.Create(fmt.Sprintf("node-%s.dot", node.GetAddr()))
				// require.NoError(t, err)

				// table.DisplayGraph(out)
			}
		}
	}
	t.Run("UDP transport",
		getTestRandom(udpFac(), nil))
	t.Run("UDP transport with a single delayed node",
		getTestRandom(udpFac(), disrupted.NewDisrupted(udpFac(), disrupted.WithFixedDelay(500*time.Millisecond))))
	t.Run("UDP transport with a single jammed node",
		getTestRandom(disrupted.NewDisrupted(udpFac(), disrupted.WithJam(2*time.Second, 3)), nil))

}

// 1-15
//
// Make every node send a broadcast message to every other nodes, with a topology made
// from two disconnected dense graphs.
// After some time, a bridge node connects both graphs.
// We then test that both sides have caught up with the other's rumors.
func Test_HW1_Integration_Broadcast_Subgraph(t *testing.T) {
	getTestDisconnected := func(commonLayer transport.Transport, disruptedLayer *disrupted.Transport) func(*testing.T) {
		disruptedLayer.SetRandomGenSeed(1)
		return func(t *testing.T) {

			// We skip any variation of this test for Windows OS due to the underlying network stack
			skipIfWIndows(t)

			nLeft := 10
			nRight := 10
			nTotal := nLeft + nRight // do not include bridge
			nodes := make([]z.TestNode, nTotal+1)
			nodesLeft := nodes[0:nLeft]
			nodesRight := nodes[nLeft:nTotal]
			nodeBridge := &(nodes[nTotal])

			// the "commonLayer" is the transport layer of all student nodes (except for the bridge)
			studentTransp := commonLayer

			antiEntropy := time.Second * 10
			ackTimeout := time.Second * 10
			waitPerNode := time.Second * 2

			for i := 0; i < nTotal; i++ {
				nodes[i] = z.NewTestNode(t, studentFac, studentTransp, "127.0.0.1:0",
					z.WithAntiEntropy(antiEntropy),
					// since everyone is sending a rumor, there is no need to have route
					// rumors
					z.WithHeartbeat(0),
					z.WithAckTimeout(ackTimeout))
			}
			*nodeBridge = z.NewTestNode(t, studentFac, disruptedLayer, "127.0.0.1:0",
				z.WithAntiEntropy(antiEntropy),
				z.WithHeartbeat(time.Hour), // we just need the initial heartbeat to be triggered
				z.WithAckTimeout(ackTimeout),
				z.WithAutostart(false),
			)

			defer terminateNodes(t, nodes)
			defer stopAllNodesWithin(t, nodes, nodesTimeout)

			// generate and apply a dense topology on both sides
			// out, err := os.Create("topology.dot")
			graph.NewGraph(1).Generate(io.Discard, nodesLeft)
			graph.NewGraph(1).Generate(io.Discard, nodesRight)

			// > make each node broadcast a rumor (no bridge yet)

			wait := sync.WaitGroup{}
			wait.Add(nTotal)
			startT := time.Now()

			for i := range nodes[:nTotal] { // ignore the bridge
				go broadcastChat(t, nodes[i], &wait, "hi from %s")
			}

			// Wait for all senders to terminate (or shortly timeout, broadcast should not be blocking)
			WaitOrTimeout(t, &wait, nodesTimeout, "timeout on node broadcast")

			// Bridge both sides
			nodeBridge.AddPeer(nodesLeft[0].GetAddr(), nodesRight[0].GetAddr())
			nodeBridge.Start()

			// Wait long enough for messages to propagate through the network
			time.Sleep(waitPerNode * time.Duration(nTotal))

			// > check that each node got all the chat messages
			wait.Add(len(nodes))
			nodesChatMsgs := make([][]*types.ChatMessage, len(nodes))
			out := new(strings.Builder)

			// fetching messages can take a bit of time with the proxy, which is why
			// we do it concurrently.
			go fetchMessages(nodes, nodesChatMsgs, out, &wait, startT)
			WaitOrTimeout(t, &wait, nodesTimeout, "timeout on message fetch") // no slow operations there

			// Display stats if a test fails
			t.Log(out.String())

			// > each node should get the same messages as the first node. We sort the
			// messages to compare them.

			expected := nodesChatMsgs[0]
			sort.Sort(types.ChatByMessage(expected))

			t.Logf("expected chat messages: %v", expected)
			require.Len(t, expected, nTotal)

			for i := 1; i < len(nodesChatMsgs); i++ {
				compare := nodesChatMsgs[i]
				sort.Sort(types.ChatByMessage(compare))

				require.Equal(t, expected, compare)
			}

			// > every node should have an entry to every other nodes in their routing
			// tables (including the bridge node)
			for _, node := range nodes {
				table := node.GetRoutingTable()
				require.Len(t, table, len(nodes), node.GetAddr())

				for _, otherNode := range nodes {
					_, ok := table[otherNode.GetAddr()]
					require.True(t, ok)
				}

				// // uncomment the following to generate the routing table graphs
				// out, err := os.Create(fmt.Sprintf("node-%s.dot", node.GetAddr()))
				// require.NoError(t, err)
				// table.DisplayGraph(out)
			}

		}
	}
	t.Run("UDP transport with a normal bridge node",
		getTestDisconnected(udpFac(), disrupted.NewDisrupted(udpFac())))
	t.Run("UDP transport with a slightly jammed bridge node",
		getTestDisconnected(udpFac(), disrupted.NewDisrupted(udpFac(), disrupted.WithJam(1*time.Second, 2))))
	t.Run("UDP transport with a heavily jammed bridge node",
		getTestDisconnected(udpFac(), disrupted.NewDisrupted(udpFac(), disrupted.WithJam(2*time.Second, 10))))
}
