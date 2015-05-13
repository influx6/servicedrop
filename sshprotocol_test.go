package servicedrop

import (
	"log"
	"testing"

	"github.com/influx6/flux"
	"golang.org/x/crypto/ssh"
)

func TestSSHProtocol(t *testing.T) {
	conf := NewRouteConfig(0, 60, func(act flux.ActionInterface) {
		act.When(func(data interface{}, _ flux.ActionInterface) {
			log.Println("We ending data:...", data)
		})
	})

	serv := PasswordSSHProtocol(conf, "io", "localhost", 2022, "/home/thelogos/.ssh/id_rsa", func(c ssh.ConnMetadata, b []byte) (*ssh.Permissions, error) {
		log.Printf("%+v %+v", c, b)
		return nil, nil
	})

	serv.Dial()
}
