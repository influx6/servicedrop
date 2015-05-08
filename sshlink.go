package servicedrop

import (
	"fmt"
	"io"
	"io/ioutil"

	// "code.google.com/p/go.crypto/ssh"
	"github.com/influx6/flux"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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

//FS creates a ftp client on the remote connection and allows file base op
func (h *SSHProtocolLink) FS() flux.ActionStackInterface {
	req := flux.NewAction()
	eo := flux.NewAction()
	st := flux.NewActionStackBy(req, eo)

	ch := req.Chain(2)

	ch.OverrideBefore(1, func(b interface{}, next flux.ActionInterface) {
		ce, ok := b.(*sftp.Client)

		if !ok {
			return
		}

		ce.Close()
	})

	cl, err := sftp.NewClient(h.conn)

	if err != nil {
		st.Complete(err)
	} else {
		st.Complete(cl)
	}

	return st
}

//Command run a given command from the remote host through the ssh.Client connection
func (h *SSHProtocolLink) Command(cmd string) flux.ActionInterface {
	return nil
}

//Request is the base level method upon which all protocolink requests are handled
//for the sake of
func (h *SSHProtocolLink) Request(path string, body io.Reader) flux.ActionStackInterface {

	req := flux.NewAction()
	err := flux.NewAction()
	st := flux.NewActionStackBy(req, err)

	if h.conn == nil {
		st.Complete(ErrorNoConnection)
		return st
	}

	act := req.Chain(2)

	act.OverrideBefore(1, func(b interface{}, next flux.ActionInterface) {
		se, ok := b.(*SSHSession)

		if !ok {
			return
		}

		se.Close()
	})

	session, er := h.conn.NewSession()

	if err != nil {
		st.Complete(er)
	} else {
		st.Complete(NewSSHSession(session, body))
	}

	return st

}
