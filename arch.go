package servicedrop

import "io"
import "code.google.com/p/go-uuid/uuid"
import "github.com/influx6/flux"

//ProtocolLinkInterface defines a ProtocolLink member method rules
type ProtocolLinkInterface interface {
	Request(string, io.Reader) flux.ActionStackInterface
	Descriptor() *ProtocolDescriptor
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

//Protocol defines the basic structure for specific service type
type Protocol struct {
	*Base
}

//NewProtocol returns a new protocol instance
func NewProtocol(desc *ProtocolDescriptor) *Protocol {
	return &Protocol{NewBase(desc)}
}
