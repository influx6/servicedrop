package servicedrop

import (
	"log"
	"testing"

	"github.com/influx6/flux"
)

func TestHTTPProLink(t *testing.T) {

	link := NewHTTPLink("io", "127.0.0.1", 80)

	if link == nil {
		t.Fatal("new link not created", link)
	}

	link.Request("/", nil).Done().Then(func(b interface{}, next flux.ActionInterface) {
		log.Println("we got req?", b)
		next.Fullfill("wind")
	}).Then(func(b interface{}, next flux.ActionInterface) {
		log.Println("1.we got to next stage?", b)
		next.Fullfill("shock")
	}).Then(func(b interface{}, next flux.ActionInterface) {
		log.Println("2.we got to next stage?", b)
		next.Fullfill("drop")
	}).Then(func(b interface{}, next flux.ActionInterface) {
		log.Println("3.we got to next stage?", b)
		next.Fullfill("flat")
	})

}
