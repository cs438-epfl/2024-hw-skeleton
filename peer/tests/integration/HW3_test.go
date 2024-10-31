package integration

import (
	"encoding/hex"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/registry/proxy"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/storage/file"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
	"go.dedis.ch/cs438/types"
)

// 3-16
//
// If there are 2 nodes but we set the TotalPeers to 3 and the threshold
// function to N, then there is no chance a consensus is reached. We then add a
// third node and a consensus should eventually be reached and name stores
// updated.
func Test_HW3_Integration_Eventual_Consensus(t *testing.T) {
	transp := channel.NewTransport()

	threshold := func(i uint) int { return int(i) }

	// Note: we are setting the antientropy on each peer to make sure all rumors
	// are spread among peers.

	// Threshold = 3

	node1 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithTotalPeers(3),
		z.WithPaxosID(1),
		z.WithPaxosThreshold(threshold),
		z.WithPaxosProposerRetry(time.Second*2),
		z.WithAntiEntropy(time.Second))
	defer node1.Stop()

	node2 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithTotalPeers(3),
		z.WithPaxosID(2),
		z.WithPaxosThreshold(threshold),
		z.WithAntiEntropy(time.Second))
	defer node2.Stop()

	// Note: we set the heartbeat and antientropy so that node3 will annonce
	// itself and get rumors.
	node3 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0",
		z.WithTotalPeers(3),
		z.WithPaxosID(3),
		z.WithPaxosThreshold(threshold),
		z.WithHeartbeat(time.Hour),
		z.WithAntiEntropy(time.Second))
	defer node3.Stop()

	node1.AddPeer(node2.GetAddr())
	node2.AddPeer(node1.GetAddr())

	tagDone := make(chan struct{})

	go func() {
		err := node1.Tag("a", "b")
		require.NoError(t, err)

		close(tagDone)
	}()

	time.Sleep(time.Second * 3)

	select {
	case <-tagDone:
		t.Error(t, "a consensus can't be reached")
	default:
	}

	// > Add a new peer: with 3 peers a consensus can now be reached. Node3 has
	// the heartbeat so it will annonce itself to node1.
	node3.AddPeer(node1.GetAddr())

	timeout := time.After(time.Second * 10)

	select {
	case <-tagDone:
	case <-timeout:
		t.Error(t, "a consensus must have been reached")
	}

	// wait for rumors to be spread, especially TLC messages.
	time.Sleep(time.Second * 3)

	// > node1 name store is updated

	names := node1.GetStorage().GetNamingStore()
	require.Equal(t, 1, names.Len())
	require.Equal(t, []byte("b"), names.Get("a"))

	// > node2 name store is updated

	names = node2.GetStorage().GetNamingStore()
	require.Equal(t, 1, names.Len())
	require.Equal(t, []byte("b"), names.Get("a"))

	// > node3 name store is updated

	names = node3.GetStorage().GetNamingStore()
	require.Equal(t, 1, names.Len())
	require.Equal(t, []byte("b"), names.Get("a"))

	time.Sleep(time.Second)

	// > all nodes must have broadcasted 1 TLC message. There could be more sent
	// if the node replied to a status from a peer that missed the broadcast.

	tlcMsgs := getTLCMessagesFromRumors(t, node1.GetOuts(), node1.GetAddr())
	require.GreaterOrEqual(t, len(tlcMsgs), 1)

	tlcMsgs = getTLCMessagesFromRumors(t, node2.GetOuts(), node2.GetAddr())
	require.GreaterOrEqual(t, len(tlcMsgs), 1)

	tlcMsgs = getTLCMessagesFromRumors(t, node3.GetOuts(), node3.GetAddr())
	require.GreaterOrEqual(t, len(tlcMsgs), 1)
}

// 3-17
//
// Given the following topology:
//
//	A -> B
//
// When A proposes a filename, then we are expecting the following message
// exchange:
//
//	A -> B: PaxosPrepare (broadcast, i.e also processed locally by A)
//
//	A -> A: PaxosPromise (broadcast-private)
//	B -> A: PaxosPromise (broadcast-private)
//
//	A -> B: PaxosPropose (broadcast, i.e also processed locally by A)
//
//	A -> A: PaxosAccept (broadcast)
//	B -> A: PaxosAccept (broadcast)
//
//	A -> B: TLC (broadcast)
//	B -> A: TLC (broadcast)
func Test_HW3_Integration_Simple_Consensus(t *testing.T) {
	transp := channel.NewTransport()

	node1 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0", z.WithTotalPeers(2), z.WithPaxosID(1))
	defer node1.Stop()

	node2 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0", z.WithTotalPeers(2), z.WithPaxosID(2))
	defer node2.Stop()

	node1.AddPeer(node2.GetAddr())

	err := node1.Tag("a", "b")
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > node1 must have sent
	//
	//   - Rumor(1):PaxosPrepare
	//   - Rumor(2):Private:PaxosPromise
	//   - Rumor(3):PaxosPropose
	//   - Rumor(4):PaxosAccept
	//   - Rumor(5):TLC

	n1outs := node1.GetOuts()

	// >> Rumor(1):PaxosPrepare

	msg, pkt := z.GetRumorWithSequence(t, n1outs, 1)
	require.NotNil(t, msg)

	require.Equal(t, node1.GetAddr(), pkt.Source)
	require.Equal(t, node1.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node2.GetAddr(), pkt.Destination)

	prepare := z.GetPaxosPrepare(t, msg)

	require.Equal(t, uint(1), prepare.ID)
	require.Equal(t, uint(0), prepare.Step)
	require.Equal(t, node1.GetAddr(), prepare.Source)

	// >> Rumor(2):Private:PaxosPromise

	msg, pkt = z.GetRumorWithSequence(t, n1outs, 2)
	require.NotNil(t, msg)

	require.Equal(t, node1.GetAddr(), pkt.Source)
	require.Equal(t, node1.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node2.GetAddr(), pkt.Destination)

	private := z.GetPrivate(t, msg)

	require.Len(t, private.Recipients, 1)
	require.Contains(t, private.Recipients, node1.GetAddr())

	promise := z.GetPaxosPromise(t, private.Msg)

	require.Equal(t, uint(1), promise.ID)
	require.Equal(t, uint(0), promise.Step)
	// default "empty" value is 0
	require.Zero(t, promise.AcceptedID)
	require.Nil(t, promise.AcceptedValue)

	// >> Rumor(3):PaxosPropose

	msg, pkt = z.GetRumorWithSequence(t, n1outs, 3)
	require.NotNil(t, msg)

	require.Equal(t, node1.GetAddr(), pkt.Source)
	require.Equal(t, node1.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node2.GetAddr(), pkt.Destination)

	propose := z.GetPaxosPropose(t, msg)

	require.Equal(t, uint(1), propose.ID)
	require.Equal(t, uint(0), propose.Step)
	require.Equal(t, "a", propose.Value.Filename)
	require.Equal(t, "b", propose.Value.Metahash)

	// >> Rumor(4):PaxosAccept

	msg, pkt = z.GetRumorWithSequence(t, n1outs, 4)
	require.NotNil(t, msg)

	require.Equal(t, node1.GetAddr(), pkt.Source)
	require.Equal(t, node1.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node2.GetAddr(), pkt.Destination)

	accept := z.GetPaxosAccept(t, msg)

	require.Equal(t, uint(1), accept.ID)
	require.Equal(t, uint(0), accept.Step)
	require.Equal(t, "a", accept.Value.Filename)
	require.Equal(t, "b", accept.Value.Metahash)

	// >> Rumor(5):TLC

	msg, pkt = z.GetRumorWithSequence(t, n1outs, 5)
	require.NotNil(t, msg)

	require.Equal(t, node1.GetAddr(), pkt.Source)
	require.Equal(t, node1.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node2.GetAddr(), pkt.Destination)

	tlc := z.GetTLC(t, msg)

	require.Equal(t, uint(0), tlc.Step)
	require.Equal(t, uint(0), tlc.Block.Index)
	require.Equal(t, make([]byte, 32), tlc.Block.PrevHash)
	require.Equal(t, "a", tlc.Block.Value.Filename)
	require.Equal(t, "b", tlc.Block.Value.Metahash)

	// > node2 must have sent
	//
	//   - Rumor(1):Private:PaxosPromise
	//   - Rumor(2):PaxosAccept
	//   - Rumor(3):TLC

	n2outs := node2.GetOuts()

	// >> Rumor(1):Private:PaxosPromise

	msg, pkt = z.GetRumorWithSequence(t, n2outs, 1)
	require.NotNil(t, msg)

	require.Equal(t, node2.GetAddr(), pkt.Source)
	require.Equal(t, node2.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node1.GetAddr(), pkt.Destination)

	private = z.GetPrivate(t, msg)

	require.Len(t, private.Recipients, 1)
	require.Contains(t, private.Recipients, node1.GetAddr())

	promise = z.GetPaxosPromise(t, private.Msg)

	require.Equal(t, uint(1), promise.ID)
	require.Equal(t, uint(0), promise.Step)
	// default "empty" value is 0
	require.Zero(t, promise.AcceptedID)
	require.Nil(t, promise.AcceptedValue)

	// >> Rumor(2):PaxosAccept

	msg, pkt = z.GetRumorWithSequence(t, n2outs, 2)
	require.NotNil(t, msg)

	require.Equal(t, node2.GetAddr(), pkt.Source)
	require.Equal(t, node2.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node1.GetAddr(), pkt.Destination)

	accept = z.GetPaxosAccept(t, msg)

	require.Equal(t, uint(1), accept.ID)
	require.Equal(t, uint(0), accept.Step)
	require.Equal(t, "a", accept.Value.Filename)
	require.Equal(t, "b", accept.Value.Metahash)

	// >> Rumor(3):TLC

	msg, pkt = z.GetRumorWithSequence(t, n2outs, 3)
	require.NotNil(t, msg)

	require.Equal(t, node2.GetAddr(), pkt.Source)
	require.Equal(t, node2.GetAddr(), pkt.RelayedBy)
	require.Equal(t, node1.GetAddr(), pkt.Destination)

	tlc = z.GetTLC(t, msg)

	require.Equal(t, uint(0), tlc.Step)
	require.Equal(t, uint(0), tlc.Block.Index)
	require.Equal(t, make([]byte, 32), tlc.Block.PrevHash)
	require.Equal(t, "a", tlc.Block.Value.Filename)
	require.Equal(t, "b", tlc.Block.Value.Metahash)

	// > node1 name store is updated

	names := node1.GetStorage().GetNamingStore()
	require.Equal(t, 1, names.Len())
	require.Equal(t, []byte("b"), names.Get("a"))

	// > node2 name store is updated

	names = node2.GetStorage().GetNamingStore()
	require.Equal(t, 1, names.Len())
	require.Equal(t, []byte("b"), names.Get("a"))

	// > node1 blockchain store contains two elements

	bstore := node1.GetStorage().GetBlockchainStore()

	require.Equal(t, 2, bstore.Len())

	lastBlockHash := bstore.Get(storage.LastBlockKey)
	lastBlock := bstore.Get(hex.EncodeToString(lastBlockHash))

	var block types.BlockchainBlock

	err = block.Unmarshal(lastBlock)
	require.NoError(t, err)

	require.Equal(t, uint(0), block.Index)
	require.Equal(t, make([]byte, 32), block.PrevHash)
	require.Equal(t, "a", block.Value.Filename)
	require.Equal(t, "b", block.Value.Metahash)

	// > node2 blockchain store contains two elements

	bstore = node2.GetStorage().GetBlockchainStore()

	require.Equal(t, 2, bstore.Len())

	lastBlockHash = bstore.Get(storage.LastBlockKey)
	lastBlock = bstore.Get(hex.EncodeToString(lastBlockHash))

	err = block.Unmarshal(lastBlock)
	require.NoError(t, err)

	require.Equal(t, uint(0), block.Index)
	require.Equal(t, make([]byte, 32), block.PrevHash)
	require.Equal(t, "a", block.Value.Filename)
	require.Equal(t, "b", block.Value.Metahash)
}

// 3-18
//
// A node joining late should be able to catchup thanks to the TLC messages. We
// check that all nodes have the same valid blockchain.
func Test_HW3_Integration_Paxos_Catchup(t *testing.T) {
	transp := channel.NewTransport()

	numBlocks := 10

	node1 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0", z.WithTotalPeers(3), z.WithPaxosID(1))
	defer node1.Stop()

	node2 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0", z.WithTotalPeers(3), z.WithPaxosID(2))
	defer node2.Stop()

	node1.AddPeer(node2.GetAddr())
	node2.AddPeer(node1.GetAddr())

	// Setting N blocks

	for i := 0; i < numBlocks; i++ {
		name := make([]byte, 12)
		rand.Read(name)

		err := node1.Tag(hex.EncodeToString(name), "metahash")
		require.NoError(t, err)
	}

	time.Sleep(time.Second)

	// > at this stage node1 and node2 must have 10 blocks in their blockchain
	// store and 10 names in their name store.

	require.Equal(t, 10, node1.GetStorage().GetNamingStore().Len())
	require.Equal(t, 10, node2.GetStorage().GetNamingStore().Len())

	// 11 for the 10 blocks and the last block's hash
	require.Equal(t, 11, node1.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 11, node2.GetStorage().GetBlockchainStore().Len())

	// > let's add the third peer and see if it can catchup.

	node3 := z.NewTestNode(t, studentFac, transp, "127.0.0.1:0", z.WithTotalPeers(3), z.WithPaxosID(3))
	defer node3.Stop()

	node3.AddPeer(node2.GetAddr())

	msg := types.EmptyMessage{}

	transpMsg, err := node3.GetRegistry().MarshalMessage(msg)
	require.NoError(t, err)

	// > by broadcasting a message node3 will get back an ack with a status,
	// making it asking for the missing rumors.

	err = node3.Broadcast(transpMsg)
	require.NoError(t, err)

	time.Sleep(time.Second * 2)

	// > checking the name and blockchain stores

	require.Equal(t, 10, node3.GetStorage().GetNamingStore().Len())
	require.Equal(t, 11, node3.GetStorage().GetBlockchainStore().Len())

	// > check that all blockchain store have the same last block hash

	blockStore1 := node1.GetStorage().GetBlockchainStore()
	blockStore2 := node2.GetStorage().GetBlockchainStore()
	blockStore3 := node3.GetStorage().GetBlockchainStore()

	require.Equal(t, blockStore1.Get(storage.LastBlockKey), blockStore2.Get(storage.LastBlockKey))
	require.Equal(t, blockStore1.Get(storage.LastBlockKey), blockStore3.Get(storage.LastBlockKey))

	// > validate the chain in each store

	z.ValidateBlockchain(t, blockStore1)

	z.ValidateBlockchain(t, blockStore2)

	z.ValidateBlockchain(t, blockStore3)
}

// 3-19
//
// Call the Tag() function on multiple peers concurrently. The state should be
// consistent for all peers.
//
// NB: this test generates _a lot_ of traffic and may easily trigger deadlocks.
// Once your implementation performs correctly, be sure to limit the amount of
// logging to improve the CI performance and keep logs readable for yourself.
func Test_HW3_Integration_Consensus_Stress_Test(t *testing.T) {
	numMessages := 7
	numNodes := 3

	transp := channel.NewTransport()

	nodes := make([]z.TestNode, numNodes)

	for i := range nodes {
		node := z.NewTestNode(
			t,
			studentFac,
			transp,
			"127.0.0.1:0",
			z.WithTotalPeers(uint(numNodes)),
			z.WithPaxosID(uint(i+1)),
			z.WithPaxosProposerRetry(time.Second*time.Duration(3+i)),
			z.WithContinueMongering(0.3))

		defer node.Stop()

		nodes[i] = node
	}

	for _, n1 := range nodes {
		for _, n2 := range nodes {
			n1.AddPeer(n2.GetAddr())
		}
	}

	wait := sync.WaitGroup{}
	wait.Add(numNodes)

	for _, node := range nodes {
		go func(n z.TestNode) {
			defer wait.Done()

			for i := 0; i < numMessages; i++ {
				name := make([]byte, 12)
				rand.Read(name)

				err := n.Tag(hex.EncodeToString(name), "1")
				require.NoError(t, err)

				time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
			}
		}(node)
	}

	wait.Wait()

	time.Sleep(time.Second * 2)

	lastHashes := map[string]struct{}{}

	for i, node := range nodes {
		t.Logf("node %d", i)

		store := node.GetStorage().GetBlockchainStore()
		require.Equal(t, numMessages*numNodes+1, store.Len())

		lastHashes[string(store.Get(storage.LastBlockKey))] = struct{}{}

		z.ValidateBlockchain(t, store)

		require.Equal(t, numMessages*numNodes, node.GetStorage().GetNamingStore().Len())

		z.DisplayLastBlockchainBlock(t, os.Stdout, node.GetStorage().GetBlockchainStore())
	}

	// > all peers must have the same last hash
	require.Len(t, lastHashes, 1)
}

// 3-20
//
// Call the Tag() function on multiple peers concurrently. The state should be
// consistent for all peers. Mixes student and reference peers.
func Test_HW3_Integration_Multiple_Consensus(t *testing.T) {
	numMessages := 5

	nStudent := 2
	nReference := 2
	totalNodes := nStudent + nReference

	referenceTransp := proxyFac()
	studentTransp := udpFac()

	nodes := make([]z.TestNode, totalNodes)

	for i := 0; i < nStudent; i++ {
		node := z.NewTestNode(t, studentFac, studentTransp, "127.0.0.1:0",
			z.WithTotalPeers(uint(totalNodes)),
			z.WithPaxosID(uint(i+1)))
		nodes[i] = node
	}

	for i := nStudent; i < totalNodes; i++ {
		tmpFolder, err := os.MkdirTemp("", "peerster_test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFolder)

		storage, err := file.NewPersistency(tmpFolder)
		require.NoError(t, err)

		node := z.NewTestNode(t, referenceFac, referenceTransp, "127.0.0.1:0",
			z.WithMessageRegistry(proxy.NewRegistry()),
			z.WithTotalPeers(uint(totalNodes)),
			z.WithPaxosID(uint(i+1)),
			z.WithStorage(storage))
		nodes[i] = node
	}

	stopNodes := func() {
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
			case <-time.After(time.Minute * 5):
				t.Error("timeout on node stop")
			}
		}()

		wait.Wait()
		close(done)
	}

	terminateNodes := func() {
		for i := 0; i < nReference; i++ {
			n, ok := nodes[i+nStudent].Peer.(z.Terminable)
			if ok {
				n.Terminate()
			}
		}
	}

	defer terminateNodes()
	defer stopNodes()

	for _, n1 := range nodes {
		for _, n2 := range nodes {
			n1.AddPeer(n2.GetAddr())
		}
	}

	wait := sync.WaitGroup{}
	wait.Add(totalNodes)

	for _, node := range nodes {
		go func(n z.TestNode) {
			defer wait.Done()

			for i := 0; i < numMessages; i++ {
				name := make([]byte, 12)
				rand.Read(name)

				err := n.Tag(hex.EncodeToString(name), "1")
				require.NoError(t, err)

				time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
			}
		}(node)
	}

	wait.Wait()

	time.Sleep(time.Second * 10)

	lastHashes := map[string]struct{}{}

	for i, node := range nodes {
		t.Logf("node %d", i)

		store := node.GetStorage().GetBlockchainStore()
		require.Equal(t, numMessages*totalNodes+1, store.Len())

		lastHashes[string(store.Get(storage.LastBlockKey))] = struct{}{}

		z.ValidateBlockchain(t, store)

		require.Equal(t, numMessages*totalNodes, node.GetStorage().GetNamingStore().Len())

		z.DisplayLastBlockchainBlock(t, os.Stdout, node.GetStorage().GetBlockchainStore())
	}

	// > all peers must have the same last hash
	require.Len(t, lastHashes, 1)
}

// -----------------------------------------------------------------------------
// Utility function(s)

// getTLCMessagesFromRumors returns the TLC messages from rumor messages. We're
// expecting the rumor message to contain only one rumor that embeds the TLC
// message. The rumor originates from the given addr.
func getTLCMessagesFromRumors(t *testing.T, outs []transport.Packet, addr string) []types.TLCMessage {
	var result []types.TLCMessage

	for _, msg := range outs {
		if msg.Msg.Type == "rumor" {
			rumor := z.GetRumor(t, msg.Msg)
			if len(rumor.Rumors) == 1 && rumor.Rumors[0].Msg.Type == "tlc" {
				if rumor.Rumors[0].Origin == addr {
					tlc := z.GetTLC(t, rumor.Rumors[0].Msg)
					result = append(result, tlc)
				}
			}
		}
	}

	return result
}
