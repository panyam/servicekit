package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	conc "github.com/panyam/gocurrent"
	gohttp "github.com/panyam/servicekit/http"
)

type TimeHandler struct {
	Fanout *conc.FanOut[gohttp.OutgoingMessage[any]]
}

// ... along with a corresponding New method
func NewTimeHandler() *TimeHandler {
	return &TimeHandler{Fanout: conc.NewFanOut[gohttp.OutgoingMessage[any]](nil)}
}

// The Validate method gates the subscribe request to see if it should be upgraded
// and if so creates the right connection type to wrap the connection
// This examples allows all upgrades and is only needed to specify the kind of
// connection type to use - in this case TimeConn.
func (t *TimeHandler) Validate(w http.ResponseWriter, r *http.Request) (out *TimeConn, isValid bool) {
	return &TimeConn{
		JSONConn: gohttp.JSONConn{
			Codec:   &gohttp.JSONCodec{},
			NameStr: "TimeConn",
		},
		handler: t,
	}, true
}

// Our TimeConn allows us to override any connection instance specific behaviours
type TimeConn struct {
	gohttp.JSONConn
	handler *TimeHandler
}

func (t *TimeConn) OnStart(conn *websocket.Conn) error {
	t.JSONConn.OnStart(conn)

	log.Println("Got a new connection.....")
	// Register the writer channel into the Fanout
	t.handler.Fanout.Add(t.Writer.InputChan(), nil, false)
	return nil
}

func (t *TimeConn) OnClose() {
	// Removal can be synchronous or asynchronous - we want to ensure it is done
	// synchronously so another publish (if one came in) wont be attempted on a closed channel
	<-t.handler.Fanout.Remove(t.Writer.InputChan(), true)
	t.JSONConn.OnClose()
}

func (t *TimeConn) OnTimeout() bool {
	return false
}

func (t *TimeConn) HandleMessage(msg any) error {
	log.Println("Received Message To Handle: ", msg)
	// sending to all listeners
	var val any = msg
	t.handler.Fanout.Send(gohttp.OutgoingMessage[any]{Data: &val})
	return nil
}

func main() {
	r := mux.NewRouter()
	timeHandler := NewTimeHandler()
	r.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		msg := r.URL.Query().Get("msg")
		var msgVal any = fmt.Sprintf("%s: %s", time.Now().String(), msg)
		timeHandler.Fanout.Send(gohttp.OutgoingMessage[any]{Data: &msgVal})
		fmt.Fprintf(w, "Published Message Successfully")
	})

	// Send the time every 1 second
	go func() {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		for {
			<-t.C
			var timeVal any = time.Now().String()
			timeHandler.Fanout.Send(gohttp.OutgoingMessage[any]{Data: &timeVal})
		}
	}()

	r.HandleFunc("/subscribe", gohttp.WSServe(timeHandler, nil))
	srv := http.Server{Handler: r}
	log.Fatal(srv.ListenAndServe())
}
