package unit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/storage"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/channel"
	"go.dedis.ch/cs438/types"
)

// 3-1
//
// If the TotalPeers is set to <= 1 then there must be no paxos messages
// exchanged.
func Test_HW3_Tag_Alone(t *testing.T) {
	transp := channel.NewTransport()

	node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer node1.Stop()

	err := node1.Tag("a", "b")
	require.NoError(t, err)

	// > no messages have been sent

	ins := node1.GetIns()
	require.Len(t, ins, 0)

	node2 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(0), z.WithPaxosID(1))
	defer node2.Stop()

	// > no messages have been sent

	ins = node2.GetIns()
	require.Len(t, ins, 0)
}

// 3-2
//
// Check that a peer does nothing if it receives a prepare message with a wrong
// step.
func Test_HW3_Paxos_Acceptor_Prepare_Wrong_Step(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	// sending a prepare with a wrong step

	prepare := types.PaxosPrepareMessage{
		Step:   99, // wrong step
		ID:     1,
		Source: proposer.GetAddress(),
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&prepare)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > acceptor must have ignored the message

	require.Len(t, acceptor.GetOuts(), 0)
	require.Equal(t, 0, acceptor.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 0, acceptor.GetStorage().GetNamingStore().Len())
}

// 3-3
//
// Check that a peer does nothing if it receives a prepare message with a wrong
// ID.
func Test_HW3_Paxos_Acceptor_Prepare_Wrong_ID(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	// sending a prepare with an ID too low

	prepare := types.PaxosPrepareMessage{
		Step:   0,
		ID:     0, // ID too low
		Source: proposer.GetAddress(),
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&prepare)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > acceptor must have ignored the message

	require.Len(t, acceptor.GetOuts(), 0)
	require.Equal(t, 0, acceptor.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 0, acceptor.GetStorage().GetNamingStore().Len())
}

// 3-4
//
// Check that a peer sends back a promise if it receives a valid prepare
// message.
func Test_HW3_Paxos_Acceptor_Prepare_Correct(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	// sending a prepare with a high ID, must then be taken into account

	prepare := types.PaxosPrepareMessage{
		Step:   0,
		ID:     99,
		Source: proposer.GetAddress(),
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&prepare)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 2)

	// > acceptor must have sent a promise

	acceptorOuts := acceptor.GetOuts()
	require.Len(t, acceptorOuts, 1)

	rumor := z.GetRumor(t, acceptorOuts[0].Msg)
	require.Len(t, rumor.Rumors, 1)

	private := z.GetPrivate(t, rumor.Rumors[0].Msg)

	require.Len(t, private.Recipients, 1)
	require.Contains(t, private.Recipients, proposer.GetAddress())

	promise := z.GetPaxosPromise(t, private.Msg)

	require.Equal(t, uint(0), promise.AcceptedID)
	require.Nil(t, promise.AcceptedValue)
	require.Equal(t, uint(99), promise.ID)
	require.Equal(t, uint(0), promise.Step)

	// > no block added

	require.Equal(t, 0, acceptor.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 0, acceptor.GetStorage().GetNamingStore().Len())
}

// 3-5
//
// Check that a peer does nothing if it receives a propose message with a wrong
// step.
func Test_HW3_Paxos_Acceptor_Propose_Wrong_Step(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	// sending a propose with a wrong step

	propose := types.PaxosProposeMessage{
		Step: 99, // wrong step
		ID:   1,
		Value: types.PaxosValue{
			Filename: "a",
			Metahash: "b",
		},
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&propose)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > acceptor must have ignored the message

	require.Len(t, acceptor.GetOuts(), 0)
	require.Equal(t, 0, acceptor.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 0, acceptor.GetStorage().GetNamingStore().Len())
}

// 3-6
//
// Check that a peer does nothing if it receives a propose message with a wrong
// ID.
func Test_HW3_Paxos_Acceptor_Propose_Wrong_ID(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	// sending a propose with a wrong ID

	propose := types.PaxosProposeMessage{
		Step: 0,
		// ID too high: 0 is expected since MaxID of a proposer starts at 0 and
		// the proposer hasn't received any prepare, so its MaxID = 0.
		ID: 2,
		Value: types.PaxosValue{
			Filename: "a",
			Metahash: "b",
		},
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&propose)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > acceptor must have ignored the message

	require.Len(t, acceptor.GetOuts(), 0)
	require.Equal(t, 0, acceptor.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 0, acceptor.GetStorage().GetNamingStore().Len())
}

// 3-7
//
// Check that if an acceptor already promised, but receives a higher ID, then it
// must return the valid promised id and promised value.
func Test_HW3_Paxos_Acceptor_Prepare_Already_Promised(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(2), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	prepare := types.PaxosPrepareMessage{
		Step: 0,
		ID:   5,
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&prepare)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// sending a propose, will make the proposer set its MaxID

	propose := types.PaxosProposeMessage{
		Step: 0,
		ID:   5,
		Value: types.PaxosValue{
			Filename: "a",
			Metahash: "b",
		},
	}

	transpMsg, err = acceptor.GetRegistry().MarshalMessage(&propose)
	require.NoError(t, err)

	header = transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > if the acceptor receives another prepare with a higher ID, it must
	// return the promise ID and promise value.

	prepare = types.PaxosPrepareMessage{
		Step: 0,
		ID:   9, // higher ID
	}

	transpMsg, err = acceptor.GetRegistry().MarshalMessage(&prepare)
	require.NoError(t, err)

	header = transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	acceptorOuts := acceptor.GetOuts()

	found := false

	// > look for the paxospromise that contains the AcceptedID and
	// AcceptedValue.
	for _, e := range acceptorOuts {
		if e.Msg.Type != "rumor" {
			continue
		}

		rumor := z.GetRumor(t, e.Msg)
		if len(rumor.Rumors) != 1 || rumor.Rumors[0].Msg.Type != "private" {
			continue
		}

		private := z.GetPrivate(t, rumor.Rumors[0].Msg)
		if private.Msg.Type != "paxospromise" {
			continue
		}

		promise := z.GetPaxosPromise(t, private.Msg)
		if promise.AcceptedValue == nil {
			continue
		}

		require.Equal(t, uint(9), promise.ID)
		require.Equal(t, uint(0), promise.Step)
		require.Equal(t, uint(5), promise.AcceptedID)
		require.Equal(t, "a", promise.AcceptedValue.Filename)
		require.Equal(t, "b", promise.AcceptedValue.Metahash)

		found = true
		break
	}

	require.True(t, found)
}

// 3-8
//
// Check that a peer broadcast an accept if it receives a propose message with a
// correct ID and correct Step.
func Test_HW3_Paxos_Acceptor_Propose_Correct(t *testing.T) {
	transp := channel.NewTransport()

	acceptor := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(1), z.WithPaxosID(1))
	defer acceptor.Stop()

	proposer, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	acceptor.AddPeer(proposer.GetAddress())

	propose := types.PaxosProposeMessage{
		Step: 0,
		ID:   0,
		Value: types.PaxosValue{
			Filename: "a",
			Metahash: "b",
		},
	}

	transpMsg, err := acceptor.GetRegistry().MarshalMessage(&propose)
	require.NoError(t, err)

	header := transport.NewHeader(proposer.GetAddress(), proposer.GetAddress(), acceptor.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = proposer.Send(acceptor.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > acceptor must have broadcasted an accept message. Must be the first
	// sent message in this case.

	acceptorOuts := acceptor.GetOuts()
	require.GreaterOrEqual(t, len(acceptorOuts), 1)

	rumor := z.GetRumor(t, acceptorOuts[0].Msg)
	require.Len(t, rumor.Rumors, 1)

	accept := z.GetPaxosAccept(t, rumor.Rumors[0].Msg)

	require.Equal(t, uint(0), accept.ID)
	require.Equal(t, uint(0), accept.Step)
	require.Equal(t, "a", accept.Value.Filename)
	require.Equal(t, "b", accept.Value.Metahash)
}

// 3-9
//
// Check that a peer does nothing if it receives a promise with a wrong step.
func Test_HW3_Paxos_Proposer_Prepare_Promise_Wrong_Step(t *testing.T) {
	transp := channel.NewTransport()

	paxosID := uint(9)

	// Two nodes needed for a consensus. Setting a special paxos ID.
	proposer := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
		z.WithPaxosProposerRetry(time.Hour),
		z.WithTotalPeers(2),
		z.WithPaxosID(paxosID))

	defer proposer.Stop()

	acceptor, err := transp.CreateSocket("127.0.0.1:0")
	require.NoError(t, err)

	proposer.AddPeer(acceptor.GetAddress())

	// making proposer propose

	go func() {
		err := proposer.Tag("name", "metahash")
		require.NoError(t, err)
	}()

	time.Sleep(time.Second)

	// > the socket must receive a paxos prepare

	packet, err := acceptor.Recv(time.Second)
	require.NoError(t, err)

	rumor := z.GetRumor(t, packet.Msg)
	require.Len(t, rumor.Rumors, 1)

	prepare := z.GetPaxosPrepare(t, rumor.Rumors[0].Msg)
	require.Equal(t, paxosID, prepare.ID)
	require.Equal(t, uint(0), prepare.Step)

	// > proposer has broadcasted the paxos prepare and sent to itself a
	// promise.

	n1outs := proposer.GetOuts()
	require.Len(t, n1outs, 2)

	// sending back a promise with a wrong step

	promise := types.PaxosPromiseMessage{
		Step: 99,
		ID:   paxosID,
	}

	transpMsg, err := proposer.GetRegistry().MarshalMessage(&promise)
	require.NoError(t, err)

	header := transport.NewHeader(acceptor.GetAddress(), acceptor.GetAddress(), proposer.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = acceptor.Send(proposer.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second)

	// > proposer must have ignored the message

	n1outs = proposer.GetOuts()

	require.Len(t, n1outs, 2)
	require.Equal(t, 0, proposer.GetStorage().GetBlockchainStore().Len())
	require.Equal(t, 0, proposer.GetStorage().GetNamingStore().Len())
}

// 3-10
//
// Check that a peer broadcast a propose when it gets enough promises.
func Test_HW3_Paxos_Proposer_Prepare_Propose_Correct(t *testing.T) {
	transp := channel.NewTransport()

	paxosID := uint(9)

	// TWO nodes needed for a consensus. Setting a special paxos ID.
	proposer := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
		z.WithPaxosProposerRetry(time.Hour),
		z.WithTotalPeers(2),
		z.WithPaxosID(paxosID),
		z.WithAckTimeout(0))

	defer proposer.Stop()

	acceptor, err := transp.CreateSocket("127.0.0.1:0")
	require.NoError(t, err)

	proposer.AddPeer(acceptor.GetAddress())

	// making proposer propose

	go func() {
		err := proposer.Tag("name", "metahash")
		require.NoError(t, err)
	}()

	time.Sleep(time.Second)

	// > the socket must receive a paxos prepare

	packet, err := acceptor.Recv(time.Second * 3)
	require.NoError(t, err)

	rumor := z.GetRumor(t, packet.Msg)
	require.Len(t, rumor.Rumors, 1)

	prepare := z.GetPaxosPrepare(t, rumor.Rumors[0].Msg)
	require.Equal(t, paxosID, prepare.ID)
	require.Equal(t, uint(0), prepare.Step)

	// sending back a correct promise

	promise := types.PaxosPromiseMessage{
		Step: 0,
		ID:   paxosID,
	}

	transpMsg, err := proposer.GetRegistry().MarshalMessage(&promise)
	require.NoError(t, err)

	header := transport.NewHeader(acceptor.GetAddress(), acceptor.GetAddress(), proposer.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = acceptor.Send(proposer.GetAddr(), packet, 0)
	require.NoError(t, err)

	go func() {
		// to fill the GetIns() array
		for {
			acceptor.Recv(0)
		}
	}()

	time.Sleep(time.Second * 3)

	// > proposer must have broadcasted a propose

	acceptorIns := acceptor.GetIns()

	proposes := getProposeMessagesFromRumors(t, acceptorIns, proposer.GetAddr())
	require.Len(t, proposes, 1)

	require.Equal(t, paxosID, proposes[0].ID)
	require.Equal(t, uint(0), proposes[0].Step)
	require.Equal(t, "name", proposes[0].Value.Filename)
	require.Equal(t, "metahash", proposes[0].Value.Metahash)
}

// 3-11
//
// If a peer doesn't receives enough TLC message it must not add a new block.
func Test_HW3_TLC_Move_Step_Not_Enough(t *testing.T) {
	transp := channel.NewTransport()

	// Threshold = 2
	node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithAckTimeout(0), z.WithTotalPeers(2))
	defer node1.Stop()

	socketX, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	node1.AddPeer(socketX.GetAddress())

	// send a first block, corresponding to the correct step

	// computed by hand
	blockHash := "14eeef32b9ede3fe4effd3b0f54c0010dcac56d0b1321ee314a8ae6043e65a18"
	previousHash := [32]byte{}

	tlc := types.TLCMessage{
		Step: 0,
		Block: types.BlockchainBlock{
			Index: 0,
			Hash:  z.MustDecode(blockHash),
			Value: types.PaxosValue{
				Filename: "a",
				Metahash: "b",
			},
			PrevHash: previousHash[:],
		},
	}

	transpMsg, err := node1.GetRegistry().MarshalMessage(&tlc)
	require.NoError(t, err)

	header := transport.NewHeader(socketX.GetAddress(), socketX.GetAddress(), node1.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = socketX.Send(node1.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 1)

	// Send a second block, but corresponding to the next step

	tlc.Step = 1

	transpMsg, err = node1.GetRegistry().MarshalMessage(&tlc)
	require.NoError(t, err)

	header = transport.NewHeader(socketX.GetAddress(), socketX.GetAddress(), node1.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = socketX.Send(node1.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 1)

	// > node1 must have nothing in its block store

	store := node1.GetStorage().GetBlockchainStore()
	require.Equal(t, 0, store.Len())
}

// 3-12
//
// If a peer receives enough TLC message it must then add a new block, and
// broadcast a TLC message (if not already done).
func Test_HW3_TLC_Move_Step_OK(t *testing.T) {
	transp := channel.NewTransport()

	// Threshold = 2
	node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithAckTimeout(0), z.WithTotalPeers(2))
	defer node1.Stop()

	socketX, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	socketY, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	node1.AddPeer(socketX.GetAddress())
	node1.AddPeer(socketY.GetAddress())

	// send two TLC messages for the same step

	// computed by hand
	blockHash := "14eeef32b9ede3fe4effd3b0f54c0010dcac56d0b1321ee314a8ae6043e65a18"
	previousHash := [32]byte{}

	tlc := types.TLCMessage{
		Step: 0,
		Block: types.BlockchainBlock{
			Index: 0,
			Hash:  z.MustDecode(blockHash),
			Value: types.PaxosValue{
				Filename: "a",
				Metahash: "b",
			},
			PrevHash: previousHash[:],
		},
	}

	transpMsg, err := node1.GetRegistry().MarshalMessage(&tlc)
	require.NoError(t, err)

	header1 := transport.NewHeader(socketX.GetAddress(), socketX.GetAddress(), node1.GetAddr())
	header2 := transport.NewHeader(socketY.GetAddress(), socketY.GetAddress(), node1.GetAddr())

	packet1 := transport.Packet{
		Header: &header1,
		Msg:    &transpMsg,
	}

	packet2 := transport.Packet{
		Header: &header2,
		Msg:    &transpMsg,
	}

	err = socketX.Send(node1.GetAddr(), packet1, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 2)

	err = socketY.Send(node1.GetAddr(), packet2, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 2)

	// > node1 must have a new block in its store

	store := node1.GetStorage().GetBlockchainStore()
	// one element is the last block hash, the other is the block
	require.Equal(t, 2, store.Len())

	blockBuf := store.Get(blockHash)

	var block types.BlockchainBlock
	err = block.Unmarshal(blockBuf)
	require.NoError(t, err)

	require.Equal(t, tlc.Block, block)

	// > node1 must have the block hash in the LasBlockKey store

	require.Equal(t, z.MustDecode(blockHash), store.Get(storage.LastBlockKey))

	// > node1 must have the name in its name store

	require.Equal(t, 1, node1.GetStorage().GetNamingStore().Len())
	require.Equal(t, []byte("b"), node1.GetStorage().GetNamingStore().Get("a"))
}

// 3-13
//
// If a peer receives TLC message for an upcoming round (step) it must keep it
// and be able to catchup once the current step is done.
func Test_HW3_TLC_Move_Step_Catchup(t *testing.T) {
	transp := channel.NewTransport()

	// Threshold = 1
	node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithAckTimeout(0), z.WithTotalPeers(1))
	defer node1.Stop()

	socketX, err := z.NewSenderSocket(transp, "127.0.0.1:0")
	require.NoError(t, err)

	node1.AddPeer(socketX.GetAddress())

	// computed by hand
	blockHash0 := "14eeef32b9ede3fe4effd3b0f54c0010dcac56d0b1321ee314a8ae6043e65a18"
	previousHash0 := [32]byte{}

	tlc0 := types.TLCMessage{
		Step: 0,
		Block: types.BlockchainBlock{
			Index: 0,
			Hash:  z.MustDecode(blockHash0),
			Value: types.PaxosValue{
				Filename: "a",
				Metahash: "b",
			},
			PrevHash: previousHash0[:],
		},
	}

	// computed by hand
	blockHash1 := "d06185eb130d8ac85528b35413984e38befabfc10d083b5ab2768eac3f4e9050"

	tlc1 := types.TLCMessage{
		Step: 1,
		Block: types.BlockchainBlock{
			Index: 1,
			Hash:  z.MustDecode(blockHash1),
			Value: types.PaxosValue{
				Filename: "e",
				Metahash: "f",
			},
			PrevHash: z.MustDecode(blockHash0),
		},
	}

	// computed by hand
	blockHash2 := "15effc3b853c253be977caa12fbe8f5d1fa4b5441ea67bba99e1496b95f1ebd5"

	tlc2 := types.TLCMessage{
		Step: 2,
		Block: types.BlockchainBlock{
			Index: 2,
			Hash:  z.MustDecode(blockHash2),
			Value: types.PaxosValue{
				Filename: "g",
				Metahash: "h",
			},
			PrevHash: z.MustDecode(blockHash1),
		},
	}

	// send for step 2

	transpMsg, err := node1.GetRegistry().MarshalMessage(&tlc2)
	require.NoError(t, err)

	header := transport.NewHeader(socketX.GetAddress(), socketX.GetAddress(), node1.GetAddr())

	packet := transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = socketX.Send(node1.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 1)

	// Send for step 1

	transpMsg, err = node1.GetRegistry().MarshalMessage(&tlc1)
	require.NoError(t, err)

	header = transport.NewHeader(socketX.GetAddress(), socketX.GetAddress(), node1.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = socketX.Send(node1.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 1)

	// > at this stage no blocks are added

	store := node1.GetStorage().GetBlockchainStore()
	require.Equal(t, 0, store.Len())

	// adding the expected TLC message. Peer must then add block 0, 1, and 2.

	transpMsg, err = node1.GetRegistry().MarshalMessage(&tlc0)
	require.NoError(t, err)

	header = transport.NewHeader(socketX.GetAddress(), socketX.GetAddress(), node1.GetAddr())

	packet = transport.Packet{
		Header: &header,
		Msg:    &transpMsg,
	}

	err = socketX.Send(node1.GetAddr(), packet, 0)
	require.NoError(t, err)

	time.Sleep(time.Second * 1)

	// > node1 must have 3 blocks in its block store

	// 3 blocks + the last block key
	require.Equal(t, 4, store.Len())
	blockBuf := store.Get(blockHash2)

	var block types.BlockchainBlock
	err = block.Unmarshal(blockBuf)
	require.NoError(t, err)

	require.Equal(t, tlc2.Block, block)

	// > node1 must have the block hash in the LasBlockKey store

	require.Equal(t, z.MustDecode(blockHash2), store.Get(storage.LastBlockKey))

	// > node1 must have 3 names in its name store

	require.Equal(t, 3, node1.GetStorage().GetNamingStore().Len())
	require.Equal(t, []byte("b"), node1.GetStorage().GetNamingStore().Get("a"))
	require.Equal(t, []byte("f"), node1.GetStorage().GetNamingStore().Get("e"))
	require.Equal(t, []byte("h"), node1.GetStorage().GetNamingStore().Get("g"))
}

// 3-14
//
// If I tag a name already taken, then the function should return an error.
func Test_HW3_Tag_Paxos_Name_Taken(t *testing.T) {
	transp := channel.NewTransport()

	// We set TotalPeers to use the consensus
	node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithAutostart(false), z.WithTotalPeers(2))

	name, metahash := "name", "metahash"

	nameStore := node1.GetStorage().GetNamingStore()
	nameStore.Set(name, []byte(metahash))

	err := node1.Tag(name, metahash)
	require.Error(t, err)
}

// 3-15
//
// If there are 2 nodes but we set the TotalPeers to 3 and the threshold
// function to N, then there is no chance a consensus is reached. If we wait 6
// seconds, and the PaxosProposerRetry is set to 4 seconds, then the proposer
// must have retried once and sent in total 2 paxos prepare.
func Test_HW3_Tag_Paxos_No_Consensus(t *testing.T) {
	transp := channel.NewTransport()

	threshold := func(i uint) int { return int(i) }

	// Threshold = 3
	node1 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0",
		z.WithTotalPeers(3),
		z.WithPaxosID(1),
		z.WithPaxosThreshold(threshold),
		z.WithPaxosProposerRetry(time.Second*4))
	defer node1.Stop()

	node2 := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", z.WithTotalPeers(3), z.WithPaxosID(2))
	defer node2.Stop()

	node1.AddPeer(node2.GetAddr())
	node2.AddPeer(node1.GetAddr())

	tagDone := make(chan struct{})
	timeout := time.After(time.Second * 6)

	go func() {
		err := node1.Tag("a", "b")

		if err == nil {
			// If Tag() was successful, close the channel to notify the select below
			close(tagDone)
		}
	}()

	var outs []transport.Packet

	select {
	case <-tagDone:
		t.Error("tag shouldn't work")
	case <-timeout:
		outs = node1.GetOuts()
	}

	// > the first rumor sent must be the paxos prepare

	msg, _ := z.GetRumorWithSequence(t, outs, 1)
	require.NotNil(t, msg)
	require.Equal(t, "paxosprepare", msg.Type)

	// > the second rumor sent must be the private rumor from A to A

	msg, _ = z.GetRumorWithSequence(t, outs, 2)
	require.NotNil(t, msg)

	private := z.GetPrivate(t, msg)
	require.Equal(t, "paxospromise", private.Msg.Type)

	// > the third rumor sent must be the second attempt with a paxos prepare

	msg, _ = z.GetRumorWithSequence(t, outs, 3)
	require.NotNil(t, msg)
	require.Equal(t, "paxosprepare", msg.Type)

	// > the fourth rumor sent must be the private rumor from A to A, in reply
	// to the second attempt

	msg, _ = z.GetRumorWithSequence(t, outs, 4)
	require.NotNil(t, msg)

	private = z.GetPrivate(t, msg)
	require.Equal(t, "paxospromise", private.Msg.Type)
}

// -----------------------------------------------------------------------------
// Utility functions

// getProposeMessagesFromRumors returns the propose messages from rumor
// messages. We're expecting the rumor message to contain only one rumor that
// embeds the propose message. The rumor originates from the given addr.
func getProposeMessagesFromRumors(t *testing.T, outs []transport.Packet, addr string) []types.PaxosProposeMessage {
	var result []types.PaxosProposeMessage

	for _, msg := range outs {
		if msg.Msg.Type == "rumor" {
			rumor := z.GetRumor(t, msg.Msg)
			if len(rumor.Rumors) == 1 && rumor.Rumors[0].Msg.Type == "paxospropose" {
				if rumor.Rumors[0].Origin == addr {
					propose := z.GetPaxosPropose(t, rumor.Rumors[0].Msg)
					result = append(result, propose)
				}
			}
		}
	}

	return result
}
