package servicedrop

import (
	"sync"
	"testing"

	"github.com/influx6/flux"
)

func TestRoute(t *testing.T) {
	r := NewRoute("apple", 2)

	r.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		if r.Payload != "red!" {
			t.Fatal("request came back with incorrect payload", r, s)
		}

	})

	r.Serve("apple", "red!")

}

func TestRouteWithPayloadPack(t *testing.T) {
	fail := flux.NewAction()
	r := NewRoute("apple", 2)
	pk := NewPayloadRack(2, fail)
	wait := new(sync.WaitGroup)

	fail.When(func(b interface{}, _ flux.ActionInterface) {
		wait.Done()
		t.Fatal("Failed to collect payload:", b)
	})

	r.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		pk, ok := r.Payload.(*PayloadRack)

		if !ok {
			t.Fatal("request payload is not a payloadrack", ok, pk, r)
			return
		}

		pk.Release().When(func(b interface{}, _ flux.ActionInterface) {
			if b != "red" {
				t.Fatal("request came back with incorrect payload", b, r, s)
			}
			wait.Done()
		})

	})

	wait.Add(1)
	pk.Load("red")
	r.Serve("apple", pk)
	wait.Wait()

}

func TestRouteWithPayloadPackFailure(t *testing.T) {
	fail := flux.NewAction()
	r := NewRoute("rack", 2)
	pk := NewPayloadRack(2, fail)
	wait := new(sync.WaitGroup)

	fail.When(func(b interface{}, _ flux.ActionInterface) {
		wait.Done()
	})

	r.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		pk, ok := r.Payload.(*PayloadRack)

		if !ok {
			t.Fatal("request payload is not a payloadrack", ok, pk, r)
			return
		}

	})

	wait.Add(1)
	pk.Load("red")
	r.Serve("rack", pk)
	wait.Wait()

}

func TestChildRoute(t *testing.T) {
	r := NewRoute("apple", 2)
	w := FromRoute(r, `{id:[\d+]}`)

	r.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		if r.Payload != "red!" {
			t.Fatal("request came back with incorrect payload", r, s)
		}

	})

	w.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		if r.Payload != "fruits!" {
			t.Fatal("request came back with incorrect payload", r, s)
		}

	})

	r.Serve("apple", "red!")
	r.Serve("apple/20", "fruits!")

}

func TestDivertRoute(t *testing.T) {
	r := NewRoute("apple", 2)
	w := InvertRoute(r, `{id:[\d+]}`)

	r.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		if r.Payload != "fruits!" {
			t.Fatal("request came back with incorrect payload", r, s)
		}

	})

	w.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		if r.Payload != "fruits!" {
			t.Fatal("request came back with incorrect payload", r, s)
		}

	})

	r.Serve("20", "fruits!")
	r.Serve("reck/300", "red!")

}
