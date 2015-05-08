package servicedrop

import (
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/influx6/flux"
)

type (

	//UDPPacket represents a udp packet information
	UDPPacket struct {
		Path    string       `json:"path"`
		Service string       `json:"service"`
		UUID    string       `json:"uuid"`
		Data    []byte       `json:"data"`
		Address *net.UDPAddr `json:"address"`
	}

	//HTTPPacket represents a resolved http request contain the body,req and res object
	//with the bodyError and ResponseError returned when making a http request
	HTTPPacket struct {
		Res           *http.Response
		Req           *http.Request
		Body          []byte
		ResponseError error
		BodyReadError error
	}

	//SSHPacket represents a base level packet sent through ssh links and servers
	SSHPacket struct {
		Cmd string
	}
)

var (
	//ErrorBadRequestType stands for errors when the interface{} recieved can not
	//be type asserted as a *http.Request object
	ErrorBadRequestType = errors.New("type is not a *http.Request")
	//ErrorBadHTTPPacketType stands for errors when the interface{} received is not a
	//bad request type
	ErrorBadHTTPPacketType = errors.New("type is not a HTTPPacket")
	//ErrorNoConnection describe when a link connection does not exists"
	ErrorNoConnection = errors.New("NoConnection")
	//ExcessSlash is a regexp handling more than one /
	ExcessSlash = regexp.MustCompile(`/+`)
	//EndSlash is a regexp for ending slashes /
	EndSlash = regexp.MustCompile(`/+$`)
)

//Sanitize cleans a text for secure uses
func Sanitize(s string) string {
	s = strings.Replace(s, "<", "&lt;", -1)
	s = strings.Replace(s, ">", "&gt;", -1)
	s = strings.Replace(s, "\r", "", -1)
	s = strings.Replace(s, "\n", "<br/>", -1)
	s = strings.Replace(s, "'", "\\'", -1)
	s = strings.Replace(s, "\b", "<backspace>", -1)
	return s
}

//NewUDPPacket creates a new udp packet
func NewUDPPacket(path, service, uuid string, data []byte, addr *net.UDPAddr) *UDPPacket {
	return &UDPPacket{
		path,
		service,
		uuid,
		data,
		addr,
	}
}

//UDPPacketFrom creates a new udp packet from a previous one with only the data
//and addr changed
func UDPPacketFrom(u *UDPPacket, data []byte, addr *net.UDPAddr) *UDPPacket {
	return NewUDPPacket(u.Path, u.Service, u.UUID, data, addr)
}

//NewHTTPPacket returns a new http packet
func NewHTTPPacket(res *http.Response, req *http.Request, e error) *HTTPPacket {
	defer res.Body.Close()
	bo, err := ioutil.ReadAll(res.Body)

	return &HTTPPacket{
		res,
		req,
		bo,
		e,
		err,
	}
}

//FluxCallback type provides a type of the generic function flux caller
type FluxCallback func(interface{}, flux.ActionInterface)

//WhenHTTPRequest returns a function that wraps an input function checking if its
//indeed a http.Request object
func WhenHTTPRequest(fx func(*http.Request, flux.ActionInterface)) FluxCallback {
	return func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*http.Request)

		if !ok {
			// next.Fullfill(ErrorBadRequestType)
			return
		}

		fx(rq, next)
	}
}

//WhenHTTPPacket returns a function that wraps an input function and check the
//return function parameters if its a HTTPPacket type
func WhenHTTPPacket(fx func(*HTTPPacket, flux.ActionInterface)) FluxCallback {
	return func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*HTTPPacket)

		if !ok {
			// next.Fullfill(ErrorBadHTTPPacketType)
			return
		}

		fx(rq, next)
	}
}
