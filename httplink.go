package servicedrop

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/influx6/flux"
)

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
	addr := fmt.Sprintf("%s:%s", h.Descriptor().Address, h.Descriptor().Port)
	url := fmt.Sprintf("%s://%s/%s", h.Descriptor().Scheme, addr, path)

	act := flux.NewAction()
	resact := flux.NewAction()
	erd := flux.NewAction()
	ad := flux.NewActDepend(act)

	ad.UseThen(func(b interface{}, r flux.ActionInterface) {
		r.Fullfill(b)
	}, resact)

	resact.Then(func(b interface{}, _ flux.ActionInterface) {
		log.Println("received next b:", b)
	})

	var req *http.Request
	var err error

	if body == nil {
		req, err = http.NewRequest("GET", url, body)

		if err != nil {
			erd.Fullfill(err)
		}

	} else {
		req, err = http.NewRequest("POST", url, body)

		if err != nil {
			erd.Fullfill(err)
		}
	}

	act.Fullfill(req)

	return flux.NewActionStackBy(ad, erd)
}
