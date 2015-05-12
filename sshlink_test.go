package servicedrop

import (
	"sync"
	"testing"

	"github.com/influx6/flux"
)

// "code.google.com/p/go.crypto/ssh"

// func TestRSALink(t *testing.T) {
// 	wc := new(sync.WaitGroup)
//
// 	rs := RSASSHProtocolLink("io", "192.30.252.0", 22, "influx6", "/home/thelogos/Lab/ssh/github")
//
// 	err := rs.Dial()
//
// 	if err != nil {
// 		t.Fatal("unable to create ssh server")
// 	}
//
// 	wc.Add(1)
//
// 	cmd := rs.Command("ls -a")
//
// 	cmd.Done().Then(func(b interface{}, next flux.ActionInterface) {
// 		wc.Done()
// 		next.Fullfill(b)
// 	})
//
// 	cmd.Error().When(func(b interface{}, next flux.ActionInterface) {
// 		wc.Done()
// 		next.Fullfill(b)
// 	})
//
// 	wc.Wait()
// 	rs.Drop()
// }

func TestPasswordLink(t *testing.T) {

	wc := new(sync.WaitGroup)
	rs := PasswordSSHProtocolLink("io", "localhost", 22, "thelogos", "Loc*20Form3")

	err := rs.Dial()

	if err != nil {
		t.Fatal("unable to create ssh server")
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
