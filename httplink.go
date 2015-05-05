package servicedrop

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/influx6/flux"
)

// var excslash = regexp.MustCompile(`/+`)

//HTTPProtocolLink handles http request connection
type HTTPProtocolLink struct {
	*ProtocolLink
	client *http.Client
}

//NewHTTPLink returns a new http protocol link
func NewHTTPLink(prefix string, addr string, port int) *HTTPProtocolLink {
	desc := NewDescriptor("http", prefix, addr, port, "0", "http")
	return &HTTPProtocolLink{
		NewProtocolLink(desc),
		new(http.Client),
	}
}

//NewHTTPSecureLink returns a new http protocol link
func NewHTTPSecureLink(prefix string, addr string, port int, trans *http.Transport) *HTTPProtocolLink {
	cl := &http.Client{Transport: trans}
	desc := NewDescriptor("http", prefix, addr, port, "0", "https")
	return &HTTPProtocolLink{
		NewProtocolLink(desc),
		cl,
	}
}

//Request is the base level method upon which all protocolink requests are handled
func (h *HTTPProtocolLink) Request(path string, body io.Reader) flux.ActionStackInterface {
	addr := fmt.Sprintf("%s:%d/%s", h.Descriptor().Address, h.Descriptor().Port, h.Descriptor().Service)

	url := fmt.Sprintf("%s://%s/%s", h.Descriptor().Scheme, addr, path)

	log.Println("path to request:", addr, url)

	red := flux.NewAction()
	erd := flux.NewAction()
	act := red.Chain(7)

	red.When(func(b interface{}, _ flux.ActionInterface) {
		log.Println("req-action fullfilled:", b)
	})

	red.Wrap().When(func(b interface{}, _ flux.ActionInterface) {
		log.Println("req-action-wrap fullfilled:", b)
	})

	act.When(func(b interface{}, _ flux.ActionInterface) {
		log.Println("depend-action fullfilled:", b)
	})

	cl := flux.NewActionStackBy(act, erd)

	var req *http.Request
	var err error

	if body == nil {
		req, err = http.NewRequest("GET", url, body)

		if err != nil {
			cl.Complete(err)
			return cl
		}

	} else {
		req, err = http.NewRequest("POST", url, body)

		if err != nil {
			cl.Complete(err)
			return cl
		}
	}

	cl.Complete(req)

	return cl
}
