package servicedrop

import (
	"strings"
	"sync"
	"time"

	"github.com/influx6/flux"
	"github.com/influx6/reggy"
)

//PayloadRack stores a payload for a specific range of time or
//instantly resolves the timeout set for the rack
//the timeout value will be multiplied by time.Millisecond so choose appropriately
//Note: If the timeout is a negative number then its seen as an immediate resolve
//else the normal process of time after is done
type PayloadRack struct {
	payload chan interface{}
	timeout time.Duration
	fail    flux.ActionInterface
	done    flux.ActionInterface
}

//Load sets the payloadrack payload
func (p *PayloadRack) Load(b interface{}) {
	go func() {
		if p.timeout <= -1 {
			go p.collect()
		} else {
			go func() {
				<-time.After(p.timeout)
				payload, ok := <-p.payload
				if ok {
					p.fail.Fullfill(payload)
				}
			}()
		}
		p.payload <- b
		close(p.payload)
	}()
}

//collect retrieves the data from the channel and fullfills the done action
func (p *PayloadRack) collect() {
	pkt, ok := <-p.payload

	if !ok {
		return
	}

	p.done.Fullfill(pkt)
}

//Release collects the payload from the payload rack
func (p *PayloadRack) Release() flux.ActionInterface {
	p.collect()
	return p.done.Wrap()
}

//NewPayloadRack returns a new instance of the payloadrack
func NewPayloadRack(timeout int, fail flux.ActionInterface) *PayloadRack {

	return &PayloadRack{
		make(chan interface{}),
		time.Duration(timeout) * time.Millisecond,
		fail,
		flux.NewAction(),
	}
}

//Sub aliases flux.Sub for use by route
// type Sub flux.Sub

func splitPattern(c string) []string {
	return strings.Split(c, "/")
}

func splitPatternAndRemovePrefix(c string) []string {
	return splitPattern(strings.TrimPrefix(c, "/"))
}

//RouteInterface defines route member rules
type RouteInterface interface {
	Serve(string, interface{})
	ServeRequest(*Request)
}

//Request represent a request payload to be sent into a route
type Request struct {
	Paths   []string
	Payload interface{}
	Param   interface{}
	Timeout int
}

//NewRequest returns a new request packet from a path and payload with an
//option param
func NewRequest(path string, pay interface{}, param interface{}, ts int) *Request {
	return &Request{
		splitPatternAndRemovePrefix(path),
		pay,
		param,
		ts,
	}
}

//FromRequest takes a request and reduces the path splice or returns
//the request if it is itself the last path
func FromRequest(r *Request, param interface{}) *Request {
	if len(r.Paths) <= 1 {
		return r
	}

	return &Request{
		r.Paths[1:],
		r.Payload,
		param,
		r.Timeout,
	}
}

//Route defines a single route path
type Route struct {
	flux.SocketInterface
	childRoutes *RouteMaker
	Path        string
	Pattern     *reggy.ClassicMatcher
	Valid       *flux.Push
	Invalid     *flux.Push
	fail        flux.ActionInterface
	DTO         int
}

//Sub decorates the Route.Valid.Subscribe with a more request friend closure caller
func (r *Route) Sub(fnx func(r *Request, s *flux.Sub)) *flux.Sub {
	return r.Valid.Subscribe(func(v interface{}, fs *flux.Sub) {
		req, ok := v.(*Request)

		if !ok {
			return
		}

		fnx(req, fs)
	})
}

//AllSub decorates the Route.Subscribe with a more request friend closure caller
func (r *Route) AllSub(fnx func(r *Request, s *flux.Sub)) *flux.Sub {
	return r.Subscribe(func(v interface{}, fs *flux.Sub) {
		req, ok := v.(*Request)

		if !ok {
			return
		}

		fnx(req, fs)
	})
}

//NotSub decorates the Route.Invalid.Subscribe with a more request friend closure caller
func (r *Route) NotSub(fnx func(r *Request, s *flux.Sub)) *flux.Sub {
	return r.Invalid.Subscribe(func(v interface{}, fs *flux.Sub) {
		req, ok := v.(*Request)

		if !ok {
			return
		}

		fnx(req, fs)
	})
}

//RawRoute returns a route struct for a specific route path
// which allows us to do NewRoute('{id:[/d+/]}')
func RawRoute(path string, base *flux.Push, ts int, fail flux.ActionInterface) *Route {
	m := reggy.GenerateClassicMatcher(path)
	r := &Route{
		base,
		nil,
		m.Original,
		m,
		nil,
		flux.PushSocket(100),
		fail,
		ts,
	}

	//add route maker for child routes
	r.childRoutes = RootRouteMaker(r, r.fail)

	//add new socket for valid routes and optional can made into payloadable
	r.Valid = flux.DoPushSocket(r, func(v interface{}, s flux.SocketInterface) {
		req, ok := v.(*Request)

		if !ok {
			return
		}

		if r.fail != nil {
			_, ok = req.Payload.(*PayloadRack)

			if !ok {

				var to int
				if req.Timeout == 0 {
					to = r.DTO
				} else {
					to = req.Timeout
				}

				py := req.Payload
				pl := NewPayloadRack(to, fail)

				req.Payload = pl
				pl.Load(py)
			}
		}

		f := req.Paths[0]

		ok = r.Pattern.Validate(f)

		if !ok {
			r.Invalid.Emit(req)
			return
		}

		req.Param = f

		s.Emit(req)
	})

	return r
}

//NewRoute returns a route struct for a specific route path
// which allows us to do NewRoute('{id:[/d+/]}')
func NewRoute(path string, buf int, ts int, fail flux.ActionInterface) *Route {
	return RawRoute(path, flux.PushSocket(buf), ts, fail)
}

//FromRoute returns a route based on a previous route
func FromRoute(r *Route, path string) *Route {
	return RawRoute(path, flux.DoPushSocket(r.Valid, func(v interface{}, s flux.SocketInterface) {
		req, ok := v.(*Request)

		if !ok {
			return
		}

		nreq := FromRequest(req, nil)

		s.Emit(nreq)
	}), r.DTO, r.fail)
}

//PatchRoute makes a route capable of creating PayloadRack route request
//by adding a fail action which dictates if routes should be made Payload Packets
//and returns the action for use
func PatchRoute(r *Route) flux.ActionInterface {
	if r.fail != nil {
		return r.fail
	}

	r.fail = flux.NewAction()
	return r.fail
}

//InvertRoute returns a route based on a previous route rejection of a request
//provides a divert like or not path logic (i.e if that path does not match the parent route)
//then this gets validate that rejected route)
//the fail action can be set as nil which then uses the previous fail action from
//the previous route
func InvertRoute(r *Route, path string, fail flux.ActionInterface) *Route {
	valids := flux.DoPushSocket(r.Invalid, func(v interface{}, s flux.SocketInterface) {
		req, ok := v.(*Request)

		if !ok {
			return
		}

		s.Emit(req)
	})

	if fail == nil {
		fail = r.fail
	}

	return RawRoute(path, valids, r.DTO, fail)
}

//Serve takes a path and a payload value to be validated by the route
func (r *Route) Serve(path string, b interface{}, timeout int) {
	if path == "" || path == "/" {
		return
	}

	r.ServeRequest(NewRequest(path, b, nil, timeout))
}

//ServeRequest takes a *Request and validates its first path (i.e path[0])
//if it matches then its validates it and sends off to its Valid socket or invalid
//socket if invalid
func (r *Route) ServeRequest(rw *Request) {
	r.Emit(rw)
}

//RouteMaker takes a long string of route paths and creates corresponding routes for each
type RouteMaker struct {
	routes  map[string]*Route
	timeout int
	fail    flux.ActionInterface
	lock    *sync.RWMutex
}

//Route retrieves the route with the id
func (r *RouteMaker) Route(m string) *Route {
	r.lock.RLock()
	w := r.routes[m]
	r.lock.RUnlock()
	return w
}

//Combine adds a route path into the route,but will excluse if that route
//already exists
func (r *RouteMaker) Combine(m, n string) {
	w, ok := r.routes[m]

	if !ok {
		return
	}

	id, _, _ := reggy.YankSpecial(n)

	_, ok = r.routes[id]

	if !ok {
		return
	}

	r.lock.Lock()
	r.routes[id] = FromRoute(w, n)
	r.lock.Unlock()
}

func makeRoute(route string, rm *RouteMaker, buf, ts int, fail flux.ActionInterface) {
	var last *Route
	parts := splitPatternAndRemovePrefix(route)

	for _, piece := range parts {
		if last == nil {
			last = NewRoute(piece, buf, ts, fail)
		} else {
			tmp := last
			last = FromRoute(tmp, piece)
		}

		rm.routes[last.Path] = last
	}
}

//NewRouteMaker returns a route maker that generates standand
//routes without the payload patch
func NewRouteMaker(route string, buf, ts int, fail flux.ActionInterface) *RouteMaker {
	store := make(map[string]*Route)

	rv := &RouteMaker{
		store,
		-1,
		fail,
		new(sync.RWMutex),
	}

	makeRoute(route, rv, buf, ts, fail)
	return rv
}

//RootRouteMaker returns a route maker with a single route in its map
//useful for when routes need internal routemakers for organization
func RootRouteMaker(root *Route, fail flux.ActionInterface) *RouteMaker {
	return &RouteMaker{
		map[string]*Route{"/": root},
		-1,
		fail,
		new(sync.RWMutex),
	}
}
