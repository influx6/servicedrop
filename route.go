package servicedrop

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/influx6/flux"
	"github.com/influx6/reggy"
)

//RouteConfig handles the initialization of protocols route
type RouteConfig struct {
	buffer  int
	timeout int
	fail    Failure
}

//NewRouteConfig returns a routeconfig with its details
func NewRouteConfig(buf, to int, fail Failure) *RouteConfig {
	return &RouteConfig{buf, to, fail}
}

//BasicRouteConfig returns a default no-op failure routeconfig
func BasicRouteConfig(buf, to int) *RouteConfig {
	return NewRouteConfig(buf, to, func(act flux.ActionInterface) {
		act.When(func(data interface{}, _ flux.ActionInterface) {})
	})
}

//Failure is a function that types behaviour to be done when using Packets and
//you want to attach a reaction to when a packet has failed usually its timeout
type Failure func(flux.ActionInterface)

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
	once    *sync.Once
}

//Load sets the payloadrack payload
func (p *PayloadRack) Load(b interface{}) {
	p.once.Do(func() {
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
	})
}

//collect retrieves the data from the channel and fullfills the done action
func (p *PayloadRack) collect() {
	pkt, ok := <-p.payload

	if !ok {
		return
	}

	p.done.Fullfill(pkt)
}

//Failed returns a packet's fail ActionInterface
func (p *PayloadRack) Failed() flux.ActionInterface {
	return p.fail.Wrap()
}

//Release collects the payload from the payload rack
func (p *PayloadRack) Release() flux.ActionInterface {
	p.collect()
	return p.done.Wrap()
}

//NewPayloadRack returns a new instance of the payloadrack
func NewPayloadRack(timeout int, fx Failure) *PayloadRack {
	p := &PayloadRack{
		make(chan interface{}),
		time.Duration(timeout) * time.Millisecond,
		flux.NewAction(),
		flux.NewAction(),
		new(sync.Once),
	}

	if fx != nil {
		fx(p.Failed())
	}

	return p
}

//Sub aliases flux.Sub for use by route
// type Sub flux.Sub

func splitPattern(c string) []string {
	return strings.Split(c, "/")
}

func trimSlash(c string) string {
	return strings.TrimPrefix(c, "/")
}

func splitPatternAndRemovePrefix(c string) []string {
	return splitPattern(trimSlash(c))
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
	childRoutes map[string]*Route
	Path        string
	Pattern     *reggy.ClassicMatcher
	Valid       *flux.Push
	Invalid     *flux.Push
	DTO         int
	lock        *sync.RWMutex
	fail        Failure
}

//New adds a new route to the current routes routemaker as a subroute
//the path string can only be a single route not a multiple
//So '/io/sucker/{f:[/w]}' will be broken down and each piece will be created
//according to its tree
//Note: '/' returns the route itself
func (r *Route) New(path string) {
	if strings.EqualFold(path, "/") || path == "" {
		return
	}

	vs := splitPatternAndRemovePrefix(path)

	if len(vs) <= 0 {
		return
	}

	fs := vs[0]

	if len(vs) > 1 {
		vs = vs[1:]
	} else {
		vs = vs[0:0]
	}

	id, _, _ := reggy.YankSpecial(fs)

	r.lock.RLock()
	d, ok := r.childRoutes[id]
	r.lock.RUnlock()

	if !ok {
		rs := FromRoute(r, fs)

		r.lock.Lock()
		r.childRoutes[rs.Path] = rs
		r.lock.Unlock()

		rs.New(strings.Join(vs, "/"))
		return
	}

	d.New(strings.Join(vs, "/"))
}

//Children returns the total child routes possed by these route
//r.childRoutes.Size() - 1 : because the child route is set to "/"
func (r *Route) Children() int {
	return len(r.childRoutes)
}

//Child checks the route routemaker if the specific childroute exits
func (r *Route) Child(path string) *Route {
	if strings.EqualFold(path, "/") || path == "" {
		return r
	}

	vs := splitPatternAndRemovePrefix(path)

	if len(vs) <= 0 {
		return nil
	}

	fs := vs[0]

	if len(vs) > 1 {
		vs = vs[1:]
	} else {
		vs = vs[0:0]
	}

	id, _, _ := reggy.YankSpecial(fs)

	r.lock.RLock()
	d, ok := r.childRoutes[id]
	r.lock.RUnlock()

	if !ok {
		return nil
	}

	return d.Child(strings.Join(vs, "/"))
}

//Sub decorates the Route.Valid.Subscribe with a more request friendly closure caller
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
//As far as operation is concerned any when a Route gets a path to validate
//against it first splits into into a list '/app/2/status' => ['app',2,'status']
//where each route within this route chain checks each first index and creates a new
//slice excluding that first index if the first value in the first index match its own
//rule and then pass it to the next route in its range else drops that packet into
//its Invalid socket, also all child routes that extend from a base will all recieve
//the parents Invalid socket so you can deal with routes that dont match in one place
func RawRoute(path string, base *flux.Push, invalid *flux.Push, ts int, fail Failure) *Route {
	if path == "" {
		panic("route path can not be an empty string")
	}

	m := reggy.GenerateClassicMatcher(path)
	r := &Route{
		base,
		make(map[string]*Route),
		m.Original,
		m,
		nil,
		invalid,
		ts,
		new(sync.RWMutex),
		fail,
	}

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
//note when using long paths eg /apple/dog/back
//this returns the root path 'apple' and not the final path 'back' route object
func NewRoute(path string, buf int, ts int, fail Failure) *Route {
	vs := splitPatternAndRemovePrefix(path)

	if len(vs) <= 0 {
		panic(fmt.Sprintf("path after split is is %v not valid", vs))
	}

	fs := vs[0]

	if len(vs) > 1 {
		vs = vs[1:]
	} else {
		vs = vs[0:0]
	}

	rw := RawRoute(fs, flux.PushSocket(buf), flux.PushSocket(buf), ts, fail)

	if len(vs) > 0 {
		rw.New(strings.Join(vs, "/"))
	}

	return rw
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
	}), flux.PushSocket(r.DTO), r.DTO, r.fail)
}

//PatchRoute makes a route capable of creating PayloadRack route request
//by adding a fail action which dictates if routes should be made Payload Packets
//and returns the action for use
func PatchRoute(r *Route, f Failure) *Route {
	if r.fail != nil {
		return r
	}

	r.fail = f
	return r
}

//InvertRoute returns a route based on a previous route rejection of a request
//provides a divert like or not path logic (i.e if that path does not match the parent route)
//then this gets validate that rejected route)
//the fail action can be set as nil which then uses the previous fail action from
//the previous route
func InvertRoute(r *Route, path string, fail Failure) *Route {
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

	return RawRoute(path, valids, flux.PushSocket(r.DTO), r.DTO, fail)
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
