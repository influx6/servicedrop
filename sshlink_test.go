package servicedrop

import (
	"sync"
	"testing"

	"github.com/influx6/flux"
)

func TestRSALink(t *testing.T) {
	wc := new(sync.WaitGroup)

	rs := RSASSHProtocolLink("io", "127.0.0.1", 22, "thelogos", "./perm/perm")

	err := rs.Dial()

	if err != nil {
		t.Fatal("unable to dial ssh server", err)
	}

	wc.Add(1)

	cmd := rs.Command("ls -a")

	cmd.Done().Then(func(b interface{}, next flux.ActionInterface) {
		wc.Done()
		next.Fullfill(b)
	})

	cmd.Error().When(func(b interface{}, next flux.ActionInterface) {
		wc.Done()
		next.Fullfill(b)
	})

	wc.Wait()
	rs.Drop()
}

func TestPasswordLink(t *testing.T) {

	wc := new(sync.WaitGroup)
	rs := PasswordSSHProtocolLink("io", "localhost", 22, "thelogos", "io")

	err := rs.Dial()

	if err != nil {
		t.Fatal("unable to create ssh server", err)
	}

	wc.Add(1)

	cmd := rs.Command("ls -a")

	cmd.Done().Then(func(b interface{}, next flux.ActionInterface) {
		wc.Done()
		next.Fullfill(b)
	})

	cmd.Error().When(func(b interface{}, next flux.ActionInterface) {
		wc.Done()
		next.Fullfill(b)
	})

	wc.Wait()
	rs.Drop()
}
