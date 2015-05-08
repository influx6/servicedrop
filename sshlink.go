package servicedrop

import (
	"fmt"
	"io"
	"io/ioutil"

	"code.google.com/p/go.crypto/ssh"
	"github.com/influx6/flux"
)

//SSHProtocolLink handles http request connection
type SSHProtocolLink struct {
	*ProtocolLink
	conf *ssh.ClientConfig
	conn *ssh.Client
}

//RSASSHProtocolLink returns a new sshProtocollink to communicate with ssh servers
//you pass
func RSASSHProtocolLink(service, addr string, port int, user string, pkeyFile string) *SSHProtocolLink {

	pbytes, err := ioutil.ReadFile(pkeyFile)

	if err != nil {
		fmt.Println("ReadError:", err)
		panic(fmt.Sprintf("Failed to load private key file: %s", pkeyFile))
	}

	private, err := ssh.ParsePrivateKey(pbytes)

	if err != nil {
		fmt.Println(fmt.Sprintf("ParseError:(%s):", pkeyFile), err)
		panic("Failed to parse private key")
	}

	auth := []ssh.AuthMethod{
		ssh.PublicKeys(private),
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: auth,
	}

	desc := NewDescriptor("ssh", service, addr, port, "0", "ssh")

	client, err := ssh.Dial("tcp", desc.Host(), config)

	if err != nil {
		panic(fmt.Sprintf("client creation received error for: %s", desc.Host()))
	}

	// desc.Misc

	nsh := &SSHProtocolLink{
		NewProtocolLink(desc),
		config,
		client,
	}

	return nsh
}

//PasswordSSHProtocolLink returns a new sshProtocollink to communicate with ssh servers
//you pass
func PasswordSSHProtocolLink(service, addr string, port int, user string, password string) *SSHProtocolLink {

	auth := []ssh.AuthMethod{
		ssh.Password(password),
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: auth,
	}

	desc := NewDescriptor("ssh", service, addr, port, "0", "ssh")
	//should we store the password?
	// desc.Misc["password"]=password

	// client, err := ssh.Dial("tcp", desc.Host(), config)
	//
	// if err != nil {
	// 	panic(fmt.Sprintf("client creation received error for: %s", desc.Host()))
	// }

	// desc.Misc

	nsh := &SSHProtocolLink{
		NewProtocolLink(desc),
		config,
		// client,
		nil,
	}

	return nsh
}

//Drop ends the ssh connection
func (h *SSHProtocolLink) Drop() error {
	if h.conn != nil {
		h.conn.Close()
	}

	return nil
}

//Dial initaites the ssh connection
func (h *SSHProtocolLink) Dial() error {
	client, err := ssh.Dial("tcp", h.Descriptor().Host(), h.conf)

	if err != nil {
		return err
		// panic(fmt.Sprintf("client creation received error for: %s", desc.Host()))
	}

	h.conn = client

	return nil

}

//Request is the base level method upon which all protocolink requests are handled
func (h *SSHProtocolLink) Request(path string, body io.Reader) flux.ActionStackInterface {
	// addr := fmt.Sprintf("%s:%d/%s", h.Descriptor().Address, h.Descriptor().Port, h.Descriptor().Service)
	// addr = ExcessSlash.ReplaceAllString(addr, "/")
	// addr = EndSlash.ReplaceAllString(addr, "")
	// path = ExcessSlash.ReplaceAllString(path, "/")
	// path = EndSlash.ReplaceAllString(path, "")
	// url := fmt.Sprintf("%s://%s/%s", h.Descriptor().Scheme, addr, path)

	req := flux.NewAction()
	err := flux.NewAction()
	st := flux.NewActionStackBy(req, err)

	if h.conn == nil {
		st.Complete(ErrorNoConnection)
		return st
	}

	act := req.Chain(2)

	act.OverrideBefore(1, func(b interface{}, next flux.ActionInterface) {
		se, ok := b.(*ssh.Session)

		if !ok {
			return
		}

		se.Close()
	})

	session, er := h.conn.NewSession()

	if err != nil {
		st.Complete(er)
	} else {
		st.Complete(session)
	}

	return st

}
