package integration

import (
	"fmt"
	"os"
	"runtime"

	"go.dedis.ch/cs438/internal/binnode"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/peer/impl"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/transport/proxy"
	"go.dedis.ch/cs438/transport/udp"
)

var studentFac peer.Factory = impl.NewPeer
var referenceFac peer.Factory

func init() {
	path := getPath()
	referenceFac = binnode.GetBinnodeFac(path)
}

// getPath returns the path in the PEER_BIN_PATH variable if set, otherwise a
// path of form ./node.<OS>.<ARCH>. For example "./node.darwin.amd64". It panics
// in case of an unsupported OS/ARCH.
func getPath() string {
	path := os.Getenv("PEER_BIN_PATH")
	if path != "" {
		return path
	}

	bin := fmt.Sprintf("./node.%s.%s", runtime.GOOS, runtime.GOARCH)

	// check if the binary exists
	_, err := os.Stat(bin)
	if err != nil {
		panic(fmt.Sprintf("unsupported OS/architecture combination: %v/%v",
			runtime.GOOS, runtime.GOARCH))
	}

	return bin
}

var udpFac transport.Factory = udp.NewUDP
var proxyFac transport.Factory = proxy.NewProxy
