package polylib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.dedis.ch/dela/core"
	delahttp "go.dedis.ch/dela/mino/proxy/http"
)

type polySent struct {
	Message  string `json:"message"`
	ToAddr   string `json:"toAddr"`
	TimeSent int64  `json:"timeSent"`
	ID       string `json:"id"`
}

type polyRecv struct {
	FromAddr string `json:"fromAddr"`
	TimeRecv int64  `json:"timeRecv"`
	ID       string `json:"id"`
}

func tryMarshal(e any) string {
	buff, err := json.Marshal(e)
	if err != nil {
		return "ERROR: marshal: " + err.Error()
	}

	return string(buff)
}

// Event defines the expected notification we receive from nodes.
type Event struct {
	Address string
	Msg     string
	ID      string
}

// Start starts the Polypus API server.
func Start(addr string, inWatch, outWatch *core.Watcher, out io.Writer) string {
	srv := delahttp.NewHTTP(addr)

	initWriter(inWatch, outWatch, out)

	srv.RegisterHandler("/recv", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)

		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		ins := watch(r.Context(), inWatch)

		for {
			select {
			case in := <-ins:
				addr := in.Address
				tt := time.Now().UnixMicro()
				msg := tryMarshal(polyRecv{FromAddr: addr, TimeRecv: tt, ID: in.ID})

				message := fmt.Sprintf(`data: %s`, msg) + "\n\n"

				_, _ = w.Write([]byte(message))
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	srv.RegisterHandler("/sent", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)

		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		outs := watch(r.Context(), outWatch)

		for {
			select {
			case out := <-outs:
				addr := out.Address
				tt := time.Now().UnixMicro()
				msg := tryMarshal(polySent{Message: out.Msg, ToAddr: addr, TimeSent: tt, ID: out.ID})

				message := fmt.Sprintf(`data: %s`, msg) + "\n\n"
				_, _ = w.Write([]byte(message))

				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	srv.RegisterHandler("/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
	})

	srv.RegisterHandler("/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
	})

	go srv.Listen()

	time.Sleep(time.Second)

	fmt.Println("🐙 Polypus server listening on", srv.GetAddr())

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c

		srv.Stop()
	}()

	return srv.GetAddr().String()
}

func newLogEvent(typ string, e Event) logEvent {
	return logEvent{
		Type: typ,
		Addr: e.Address,
		Time: time.Now().UnixMicro(),
		ID:   e.ID,
		Msg:  e.Msg,
	}
}

type logEvent struct {
	Type string `json:"type"`
	Addr string `json:"addr"`
	Time int64  `json:"time"`
	ID   string `json:"id"`
	Msg  string `json:"msg"`
}

func initWriter(inWatch, outWatch *core.Watcher, out io.Writer) {
	if out == nil {
		return
	}

	ins := watch(context.Background(), inWatch)
	go func() {
		for event := range ins {
			// we write to the buffer as a best effort
			_, _ = out.Write([]byte(tryMarshal(newLogEvent("recv", event)) + "\n"))
		}
	}()

	outs := watch(context.Background(), outWatch)
	go func() {
		for event := range outs {
			// we write to the buffer as a best effort
			_, _ = out.Write([]byte(tryMarshal(newLogEvent("send", event)) + "\n"))
		}
	}()
}

// observer defines an observer to fill a channel
//
// - implements core.Observer
type observer struct {
	ch chan Event
}

// NotifyCallback implements core.Observer. It drops the message if the channel
// is full.
func (o observer) NotifyCallback(e interface{}) {
	select {
	case o.ch <- e.(Event):
	default:
		fmt.Println("channel full")
	}
}

// watch implements core.Observer
func watch(ctx context.Context, watcher core.Observable) <-chan Event {
	obs := observer{ch: make(chan Event, 1000)}

	watcher.Add(obs)

	go func() {
		<-ctx.Done()
		watcher.Remove(obs)
		close(obs.ch)
	}()

	return obs.ch
}
