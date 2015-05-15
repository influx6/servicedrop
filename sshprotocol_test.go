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

	var serv *SSHProtocol

	serv = PasswordSSHProtocol(conf, "io", "localhost", 2022, "/home/thelogos/.ssh/id_rsa", PasswordAuthenticationWrap(func(p ProtocolInterface, c ssh.ConnMetadata, b []byte) (*ssh.Permissions, error) {
		log.Println("Authenticate: ...", p, c, b)
		return nil, nil
	}, serv))

	prox := BaseProxySSHProtocol(serv)

	if prox == nil {
		t.Fatal("unable to create proxy server off current ssh-server")
	}

	env := serv.Routes().Child("session/env")
	env.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving env request:", data.Paths)
	})

	exec := serv.Routes().Child("session/exec")
	exec.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving exec-req request:", data.Paths)
	})

	shell := serv.Routes().Child("session/shell")
	shell.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving shell request:", data.Paths)
	})

	serv.Dial()
}
