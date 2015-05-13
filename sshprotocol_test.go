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
		log.Printf("Authenticate: ...")
		return nil, nil
	})

	env := serv.Routes().Child("session/env")
	env.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving env request")
	})

	pty := serv.Routes().Child("session/pty-req")
	pty.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving pty-req request")
	})

	exec := serv.Routes().Child("session/exec")
	exec.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving exec-req request")
	})

	shell := serv.Routes().Child("session/shell")
	shell.Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving shell request")
	})

	serv.Dial()
}
