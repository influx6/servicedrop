package servicedrop

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"testing"
	"time"
)

type (
	Conver func(http.ResponseWriter, *http.Request)
)

func MakeListener(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

func MakeBaseServer(l net.Listener, handle Conver) *http.Server {
	log.Printf("Using net.Listener with Addr %+s", l.Addr().String())

	s := &http.Server{
		Addr:           l.Addr().String(),
		Handler:        http.HandlerFunc(handle),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.SetKeepAlivesEnabled(true)
	go s.Serve(l)

	return s
}

func MakeConnections(from, to int, t *testing.T) {
	tourl := fmt.Sprintf("127.0.0.1:%d", to)
	do, err := MakeListener(from)

	if err != nil {
		t.Fatalf("Unable to create to server %+s", err)
	}

	go func() {

		log.Printf("Proccessing connections from %s to %d", do.Addr().String(), to)

		req, err := do.Accept()

		if err != nil {
			log.Printf("Accepting Error: %+s", err)
		}

		res, err := net.Dial("tcp", tourl)

		prx := TCPStream(req, res)

		log.Println("Created proxy:", prx)

	}()
}

func TestProxyWithRealServer(t *testing.T) {

	from, err := MakeListener(3030)

	if err != nil {
		t.Fatalf("Unable to create from server %+s", err)
	}

	frs := MakeBaseServer(from, func(res http.ResponseWriter, req *http.Request) {
		url := req.URL.String()
		res.Write([]byte(fmt.Sprintf("Welcome %s to your back server!", url)))
	})

	log.Println("Created Server:", frs.Addr)

	MakeConnections(7070, 3030, t)

	res, err := http.Get("http://127.0.0.1:7070")

	if err != nil {
		t.Fatalf("Error occured: %+s", err)
	}

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		t.Fatalf("Error occured while reading body: %+s", err)
	}

	t.Logf("Body response: %s", body)
}
