package main

import (
	"log"
	"net/http"
	"os"

	"github.com/livetemplate/livetemplate"
	e2etest "github.com/livetemplate/lvt/testing"
)

// sharedAuth puts every connection in the same session group, so
// `ctx.BroadcastAction` from any tab/embed reaches everyone else.
// Real apps use a per-user authenticator; here a constant groupID is
// what makes the literate-linked tutorial demonstrate cross-region
// state sharing.
type sharedAuth struct{}

func (sharedAuth) Identify(r *http.Request) (string, error) {
	return "shared", nil
}

func (sharedAuth) GetSessionGroup(r *http.Request, userID string) (string, error) {
	return "shared", nil
}

func main() {
	tmpl := livetemplate.Must(livetemplate.New("counter",
		livetemplate.WithParseFiles("counter.tmpl"),
		livetemplate.WithAuthenticator(sharedAuth{}),
		// Tinkerdown's reverse-proxy rewrites Host to localhost:9090 but
		// leaves the browser's Origin header as the docs origin
		// (devbox:8083 / 8084). Default same-origin check would reject
		// these as cross-origin. For a tutorial counter that's deployed
		// on the same trusted infra as the docs site, permissive is
		// the right posture.
		livetemplate.WithPermissiveOriginCheck(),
	))
	handler := tmpl.Handle(&CounterController{}, livetemplate.AsState(&Counter{}))

	mux := http.NewServeMux()
	mux.HandleFunc("/livetemplate-client.js", e2etest.ServeClientLibrary)
	mux.HandleFunc("/livetemplate.css", e2etest.ServeCSS)
	mux.Handle("/", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}
	log.Printf("counter listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
