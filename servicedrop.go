package servicedrop

import (
	"github.com/influx6/flux"
)

type (

	//Discovery interface defines the member method rules for a discovery master service
	//for services built with service drop,it is an service registry and distributor
	Discovery interface {
		Discover(string) flux.ActionInterface
		Register(string, ProtocolDescriptor) flux.ActionInterface
		UnRegister(string) flux.ActionInterface
	}
)
