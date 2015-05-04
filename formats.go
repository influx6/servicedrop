package servicedrop

import "net"

//UDPPacket represents a udp packet information
type UDPPacket struct {
	Path    string       `json:"path"`
	Service string       `json:"service"`
	UUID    string       `json:"uuid"`
	Data    []byte       `json:"data"`
	Address *net.UDPAddr `json:"address"`
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
