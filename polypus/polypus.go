package main

// Package main provides a script that runs N nodes and exposes their Polypus
// APIs. It is an interactive script that allows to broadcast messages and tag
// files.
//
// On can specify the number of peers with the environment variable NUM_PEERS,
// and a destination file for the log with the LOG_FILE. For example:
//
//  LOG_FILE=logs.json NUM_PEERS=10 go run .
//
import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"

	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"go.dedis.ch/cs438/internal/graph"
	z "go.dedis.ch/cs438/internal/testing"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/polypus/lib/polytransp"
	"go.dedis.ch/cs438/registry/standard"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
)

const defaultNumPeers = 4

var defaultRegistry = standard.NewRegistry()
var t = testing{}

func main() {
	numPeers := getNumPeers()

	fmt.Printf("Using %d number of peers\n", numPeers)

	peerFac := impl.NewPeer
	peers := make([]z.TestNode, numPeers)

	transp := polytransp.NewTransport()

	opts := []z.Option{
		z.WithAckTimeout(time.Second * 30),
		z.WithHeartbeat(0),
		z.WithTotalPeers(uint(numPeers)),
		z.WithPaxosProposerRetry(time.Second * 60),
	}

	wg := sync.WaitGroup{}
	wg.Add(numPeers)

	for i := range peers {
		go func(i int) {
			defer wg.Done()
			antiEntropyOpt := z.WithAntiEntropy(time.Second * time.Duration(5+rand.Intn(5)))
			opts := append(opts, z.WithPaxosID(uint(i+1)))
			opts = append(opts, antiEntropyOpt)
			node := z.NewTestNode(t, peerFac, transp, "127.0.0.1:0", opts...)

			peers[i] = node
		}(i)
	}

	wg.Wait()

	graph := graph.NewGraph(0.3)
	graph.Generate(io.Discard, peers)

	time.Sleep(time.Millisecond * 200)
	polyConfig := transp.GetConfig()
	fmt.Printf("🐙 config:\n\n%s\n\n", polyConfig)

	prompt := &survey.Select{
		Message: "What do you want to do ?",
		Options: []string{
			"💬 Send a chat message",
			"🏷 Tag something",
			"👉 exit",
		},
	}

	var action string

	for {
		err := survey.AskOne(prompt, &action)
		if err != nil {
			fmt.Println(err)
			return
		}

		switch action {
		case "💬 Send a chat message":
			err = chat(peers)
			if err != nil {
				log.Fatalf("failed to chat: %v", err)
			}
		case "🏷 Tag something":
			err = tag(peers)
			if err != nil {
				log.Fatalf("failed to tag: %v", err)
			}
		case "👉 exit":
			fmt.Println("bye 👋")
			os.Exit(0)
		}
	}
}

func chat(peers []z.TestNode) error {
	addrs := make([]string, len(peers))
	for i, n := range peers {
		addrs[i] = n.GetAddr()
	}

	answers := struct {
		PeerID    uint
		Message   string
		Broadcast bool
		Recipient string
	}{}

	peerIDValidator := func(ans interface{}) error {
		str, _ := ans.(string)

		peerID, err := strconv.Atoi(str)
		if err != nil || peerID < 0 || peerID >= len(peers) {
			return xerrors.Errorf("please enter a number 0 < N < %d", len(peers))
		}

		return nil
	}

	err := survey.Ask([]*survey.Question{
		{
			Name:     "peerID",
			Prompt:   &survey.Input{Message: fmt.Sprintf("Enter the peedID, from 0 to %d", len(peers)-1)},
			Validate: peerIDValidator,
		},
		{
			Name:   "message",
			Prompt: &survey.Input{Message: "Enter your message"},
		},
	}, &answers)

	if err != nil {
		return xerrors.Errorf("failed to get the answers: %v", err)
	}

	err = survey.AskOne(&survey.Confirm{Message: "Do you want to broadcast?"}, &answers.Broadcast)
	if err != nil {
		return xerrors.Errorf("failed to get the confirmation: %v", err)
	}

	if !answers.Broadcast {
		err = survey.AskOne(&survey.Select{Message: "Select the recipient", Options: addrs}, &answers.Recipient)
		if err != nil {
			return xerrors.Errorf("failed to select recipient: %v", err)
		}
	}

	chatMsg := types.ChatMessage{
		Message: answers.Message,
	}

	transpMsg, err := defaultRegistry.MarshalMessage(chatMsg)
	if err != nil {
		return xerrors.Errorf("failed to marshal message: %v", err)
	}

	fmt.Printf("Sending message %q, broadcast: %v, recipient: %s\n", answers.Message, answers.Broadcast, answers.Recipient)

	var confirm bool

	err = survey.AskOne(&survey.Confirm{Message: "Confirm?"}, &confirm)
	if err != nil {
		return xerrors.Errorf("failed to get the confirmation: %v", err)
	}

	if !confirm {
		fmt.Println("abort")
		return nil
	}

	if answers.Broadcast {
		err = peers[answers.PeerID].Broadcast(transpMsg)
		if err != nil {
			return xerrors.Errorf("failed to broadcast: %v", err)
		}
		return nil
	}

	err = peers[answers.PeerID].Unicast(answers.Recipient, transpMsg)
	if err != nil {
		return xerrors.Errorf("failed to unicast: %v", err)
	}

	return nil
}

func tag(peers []z.TestNode) error {
	answers := struct {
		PeerID   uint
		Tag      string
		Metahash string
	}{}

	peerIDValidator := func(ans interface{}) error {
		str, _ := ans.(string)

		peerID, err := strconv.Atoi(str)
		if err != nil || peerID < 0 || peerID >= len(peers) {
			return xerrors.Errorf("please enter a number 0 < N < %d", len(peers))
		}

		return nil
	}

	err := survey.Ask([]*survey.Question{
		{
			Name:     "peerID",
			Prompt:   &survey.Input{Message: fmt.Sprintf("Enter the peedID, from 0 to %d", len(peers)-1)},
			Validate: peerIDValidator,
		},
		{
			Name:   "tag",
			Prompt: &survey.Input{Message: "Enter the tag name"},
		},
		{
			Name:   "metahash",
			Prompt: &survey.Input{Message: "Enter the metahash"},
		},
	}, &answers)

	if err != nil {
		return xerrors.Errorf("failed to get the answers: %v", err)
	}

	fmt.Printf("Tag %q for metahash %q on Peer n°%d\n", answers.Tag, answers.Metahash, answers.PeerID)

	var confirm bool

	err = survey.AskOne(&survey.Confirm{Message: "Confirm?"}, &confirm)
	if err != nil {
		return xerrors.Errorf("failed to get the confirmation: %v", err)
	}

	if !confirm {
		fmt.Println("abort")
		return nil
	}

	done := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		defer close(done)
		err = peers[answers.PeerID].Tag(answers.Tag, answers.Metahash)
		if err != nil {
			fmt.Printf("failed to tag: %v\n", err)
		}
	}()

	select {
	case <-done:
		fmt.Println("tagged!")
	case <-c:
		fmt.Println("cancel")
	}

	return nil
}

func getNumPeers() int {
	n, err := strconv.Atoi(os.Getenv("NUM_PEERS"))
	if err != nil {
		return defaultNumPeers
	}

	return n
}

// testing provides a simple implementation of the require.Testing interface.
// Needed because we use some the the testing utility functions.
type testing struct{}

func (testing) Errorf(format string, args ...interface{}) {
	fmt.Println("~~ERROR~~")
	fmt.Printf(format, args...)
}

func (testing) FailNow() {
	os.Exit(1)
}
