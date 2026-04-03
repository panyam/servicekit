package http

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

// ExampleWSServe demonstrates the basic pattern for creating a WebSocket
// endpoint. The handler validates connections (auth, etc.) and WSServe
// manages the lifecycle (upgrade, heartbeats, message dispatch, cleanup).
func ExampleWSServe() {
	r := mux.NewRouter()
	r.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Publishing Custom Message")
	})

	r.HandleFunc("/subscribe", WSServe(&JSONHandler{}, nil))
	srv := http.Server{Handler: r}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
