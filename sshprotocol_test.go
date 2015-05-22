package servicedrop

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/influx6/flux"
	"golang.org/x/crypto/ssh"
)

func TestRSASSHProxyProtocolCreation(t *testing.T) {
	conf := NewRouteConfig(0, 60, func(act flux.ActionInterface) {
		act.When(func(data interface{}, _ flux.ActionInterface) {
			log.Println("We ending data:...", data)
		})
	})

	var serv *SSHProtocol

	serv = RSASSHProtocol(conf, "io", "localhost", 2022, "./perm/perm", KeyAuthenticationWrap(func(p ProtocolInterface, c ssh.ConnMetadata, b ssh.PublicKey) (*ssh.Permissions, error) {
		log.Println("Authenticate: ...", p, c, b)
		return nil, nil
	}, serv))

	prox := BaseProxySSHProtocol(serv)

	if prox == nil {
		t.Fatal("unable to create proxy server off current ssh-server")
	}

	AddRedirectBehaviour(prox.SSHProtocol)

	prox.Drop()
}

func TestSSHProxyProtocolCreation(t *testing.T) {
	conf := NewRouteConfig(0, 60, func(act flux.ActionInterface) {
		act.When(func(data interface{}, _ flux.ActionInterface) {
			log.Println("We ending data:...", data)
		})
	})

	var serv *SSHProtocol

	serv = PasswordSSHProtocol(conf, "io", "localhost", 2022, "./perm/perm", PasswordAuthenticationWrap(func(p ProtocolInterface, c ssh.ConnMetadata, b []byte) (*ssh.Permissions, error) {
		log.Println("Authenticate: ...", p, c, b)
		return nil, nil
	}, serv))

	prox := BaseProxySSHProtocol(serv)

	if prox == nil {
		t.Fatal("unable to create proxy server off current ssh-server")
	}

	AddRedirectBehaviour(prox.SSHProtocol)

	prox.Drop()
}

func TestSSHProtocol(t *testing.T) {
	conf := NewRouteConfig(0, 60, func(act flux.ActionInterface) {
		act.When(func(data interface{}, _ flux.ActionInterface) {
			log.Println("We ending data:...", data)
		})
	})

	var serv *SSHProtocol

	serv = PasswordSSHProtocol(conf, "io", "localhost", 2022, "./perm/perm", PasswordAuthenticationWrap(func(p ProtocolInterface, c ssh.ConnMetadata, b []byte) (*ssh.Permissions, error) {
		log.Println("Authenticate: ...", p, c, b)
		return nil, nil
	}, serv))

	AddExecBehaviour(serv)
	AddPtyBehaviour(serv)
	AddShellBehaviour(serv)

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

	go func() {
		<-time.After(time.Duration(60) * time.Millisecond)
		// serv.Drop()
		os.Exit(0)
	}()

	serv.Dial()

}
