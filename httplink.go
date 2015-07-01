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
	log.Printf("Initiating HTTPLink: New HTTP Request for %s", path)
	addr := fmt.Sprintf("%s:%d/%s", h.Descriptor().Address, h.Descriptor().Port, h.Descriptor().Service)
	addr = ExcessSlash.ReplaceAllString(addr, "/")
	addr = EndSlash.ReplaceAllString(addr, "")
	path = ExcessSlash.ReplaceAllString(path, "/")
	path = EndSlash.ReplaceAllString(path, "")
	resuri := fmt.Sprintf("%s/%s", addr, path)
	resuri = ExcessSlash.ReplaceAllString(resuri, "/")
	url := fmt.Sprintf("%s://%s", h.Descriptor().Scheme, resuri)

	log.Printf("HTTPLink: New HTTP Request for %s", url)

	red := flux.NewAction()
	erd := flux.NewAction()

	act := red.Chain(3)

	//we do these so we can override what happens after the user adds all they
	//want on the request
	act.OverrideBefore(1, func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*http.Request)

		if !ok {
			// next.Fullfill(b interface)
			return
		}

		rq.Header.Set("X-Service-Request", h.Descriptor().Service)

		log.Printf("HTTPLink: Request Header Set,Initiaing RequestClient Sending...")
		res, err := h.client.Do(rq)

		if err == nil {
			log.Printf("HTTPLink: Recieved Request (%s) Response Status: %d", rq.URL, res.StatusCode)
		} else {
			log.Printf("HTTPLink: Recieved Request (%s) Response Error: %v", rq.URL, err)
		}

		pck := NewHTTPPacket(res, rq, err)

		next.Fullfill(pck)
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

	log.Printf("HTTPLink: Initiating Http Request Method: (%+s), URI: (%+s)", req.Method, req.URL)

	cl.Complete(req)

	return cl
}
