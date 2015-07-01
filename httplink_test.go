package servicedrop

import (
	"net/http"
	"testing"

	"github.com/influx6/flux"
)

func TestHTTPProLink(t *testing.T) {

	link := NewHTTPLink("", "127.0.0.1", 8080)

	if link == nil {
		t.Fatal("new link not created", link)
	}

	ax := link.Request("/io", nil).Done().Then(WhenHTTPRequest(func(req *http.Request, next flux.ActionInterface) {
		req.Header.Set("X-WE-Wrote-IT", "1")
		next.Fullfill(req)
	}))

	pck := <-ax.Sync(2)

	if _, ok := pck.(*HTTPPacket); !ok {
		t.Fatal("Returned value is not a HTTPPacket:", ok, pck, ax)
	}

}
