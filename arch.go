package servicedrop

import (
	"fmt"
	"io"

	"code.google.com/p/go-uuid/uuid"
	"github.com/influx6/flux"
)

//ProtocolLinkInterface defines a ProtocolLink member method rules
type ProtocolLinkInterface interface {
	Request(string, io.Reader) flux.ActionStackInterface
	Descriptor() *ProtocolDescriptor
	Dial() error
	Drop() error
}

//Drop allows the ending if possible of a connection
func (p *ProtocolLink) Drop() error {
	return nil
}

//Dial allows the initiation of a protocol link connection
func (p *ProtocolLink) Dial() error {
	return nil
}

//ProtocolDescriptor describes a protocol identity
type ProtocolDescriptor struct {
	Service string                 `json:"service"`
	Address string                 `json:"address"`
	Port    int                    `json:"port"`
	Zone    string                 `json:"zone"`
	Scheme  string                 `json:"scheme"`
	Misc    map[string]interface{} `json:"misc"`
	Proto   string                 `json:"proto"`
	UUID    string                 `json:"uuid"`
}

//NewDescriptor creates a new ProtocolDescriptor
func NewDescriptor(proto string, name string, addr string, port int, zone string, scheme string) *ProtocolDescriptor {
	return &ProtocolDescriptor{
		name,
		addr,
		port,
		zone,
		scheme,
		make(map[string]interface{}),
		proto,
		uuid.New(),
	}
}

//Host returns the combination of address and port in 'Addr:Port' format
func (d *ProtocolDescriptor) Host() string {
	return fmt.Sprintf("%s:%d", d.Address, d.Port)
}

//Base describes a shared type by protocols and links
type Base struct {
	descriptor *ProtocolDescriptor
}

//Descriptor return the internal link descriptor
func (p *Base) Descriptor() *ProtocolDescriptor {
	return p.descriptor
}

//NewBase returns a new base with the LinkDescriptor
func NewBase(desc *ProtocolDescriptor) *Base {
	return &Base{desc}
}

//ProtocolLink provides a connection to a protocol service
type ProtocolLink struct {
	*Base
}

//Request sends off a request to the link
func (p *ProtocolLink) Request(path string, body io.Reader) flux.ActionInterface {
	return nil
}

//NewProtocolLink returns a protocol with the specific descriptor
func NewProtocolLink(desc *ProtocolDescriptor) *ProtocolLink {
	return &ProtocolLink{NewBase(desc)}
}

//ProtocolInterface represent the protocl interface member method rules
type ProtocolInterface interface {
	Routes() *Route
	Descriptor() *ProtocolDescriptor
	Sessions() SessionManagerInterface
	Drop() error
	Dial() error
}

//Protocol defines the basic structure for specific service type
type Protocol struct {
	*Base
	ProtocolClosed chan struct{}
	sessions       *SessionManager
	conf           *RouteConfig
	routes         *Route
	NetworkOpen    flux.Pipe
	NetworkClose   flux.Pipe
	NetworkCycle   flux.Pipe
}

//Drop drops the protocol connection
func (p *Protocol) Drop() error {
	return nil
}

//Dial connects the protocol connection
func (p *Protocol) Dial() error {
	return nil
}

//Routes represent the internal router used by protocols
func (p *Protocol) Routes() *Route {
	return p.routes
}

//Sessions returns the sessionmanager for this protocol
func (p *Protocol) Sessions() SessionManagerInterface {
	return p.sessions
}

//BaseProtocol returns a new protocol instance
func BaseProtocol(desc *ProtocolDescriptor, rc *RouteConfig) *Protocol {
	return &Protocol{
		NewBase(desc),
		make(chan struct{}),
		NewSessionManager(),
		rc,
		NewRoute(desc.Service, rc.buffer, rc.timeout, rc.fail),
		flux.PushSocket(0),
		flux.PushSocket(0),
		flux.PushSocket(0),
	}
}
