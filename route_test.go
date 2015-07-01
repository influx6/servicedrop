package servicedrop

import (
	"sync"
	"testing"

	"github.com/influx6/flux"
)

func TestRoute(t *testing.T) {
	r := NewRoute("apple", 2, 0, nil)

	r.Sub(func(r *Request, s *flux.Sub) {
		defer s.Close()

		if r.Payload != "red!" {
			t.Fatal("request came back with incorrect payload", r, s)
		}

	})

	r.Serve("apple", "red!", 0)

}

func TestRouteWithPayloadPack(t *testing.T) {
	wait := new(sync.WaitGroup)
	r := NewRoute("apple", 2, 3, func(fail flux.ActionInterface) {
		if fail == nil {
			t.Fatal("Unable to affect failure action")
		}

		fail.When(func(b interface{}, _ flux.ActionInterface) {
			wait.Done()
		})
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
	r.Serve("apple", "red", 2)
	wait.Wait()

}

func TestRouteWithPayloadPackFailure(t *testing.T) {
	wait := new(sync.WaitGroup)
	r := NewRoute("rack", 2, 3, func(fail flux.ActionInterface) {
		if fail == nil {
			t.Fatal("Unable to affect failure action")
		}

		fail.When(func(b interface{}, _ flux.ActionInterface) {
			wait.Done()
		})
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
	r.Serve("rack", "red", 2)
	wait.Wait()

}

func TestChildRoute(t *testing.T) {
	r := NewRoute("apple", 2, 0, nil)
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

	r.Serve("apple", "red!", 0)
	r.Serve("apple/20", "fruits!", 0)

}

func TestDivertRoute(t *testing.T) {
	r := NewRoute("apple", 2, 0, nil)
	w := InvertRoute(r, `{id:[\d+]}`, nil)

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

	r.Serve("20", "fruits!", 0)
	r.Serve("reck/300", "red!", 0)

}

func TestRouteMaker(t *testing.T) {
	r := NewRoute("apple", 2, 0, nil)

	r.New("red/thunder")

	if r.Children() > 2 {
		t.Fatalf("Route maker with route %s has only %d", r.Path, r.Children())
	}

	red := r.Child("red")

	if red == nil {
		t.Fatal("/apple/red route does not exists:", red, r)

		if red.Path != "red" {
			t.Fatalf("found /dog route but path is: %s", red.Path)
		}
	}

}
