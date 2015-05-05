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
	fail := flux.NewAction()
	r := NewRoute("apple", 2, 3, fail)
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
	r.Serve("apple", "red", 2)
	wait.Wait()

}

func TestRouteWithPayloadPackFailure(t *testing.T) {
	fail := flux.NewAction()
	r := NewRoute("rack", 2, 3, fail)
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

func TestRootRouteMaker(t *testing.T) {
	r := NewRoute("apple", 2, 0, nil)

	r.New("red")

	if r.Children() > 1 {
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

func TestRouteMaker(t *testing.T) {
	path := `/dogs/{amt:[\w+]}`
	maker := NewRouteMaker(path, 2, 0, nil)

	if maker.Size() < 2 {
		t.Fatalf("Route maker with route %s has only %d", path, maker.Size())
	}

	dogs := maker.Route("dogs")

	if dogs == nil {
		t.Fatal("/dogs route does not exists:", path, dogs, maker)

		if dogs.Path != "dogs" {
			t.Fatalf("found /dog route but path is: %s", dogs.Path)
		}
	}

	amt := maker.Route("amt")

	if amt == nil {
		t.Fatal("/dog/amt route does not exists:", path, amt, maker)
		if amt.Path != "amt" {
			t.Fatalf("found /dog route but path is: %s", amt.Path)
		}
	}

}
