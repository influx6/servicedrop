package servicedrop

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"

	"github.com/influx6/flux"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

//SSHProtocolLink handles http request connection
type SSHProtocolLink struct {
	*ProtocolLink
	conf *ssh.ClientConfig
	conn *ssh.Client
	done flux.ActionInterface
}

//RSASSHProtocolLink returns a new sshProtocollink to communicate with ssh servers
//you pass
func RSASSHProtocolLink(service, addr string, port int, user string, pkeyFile string) *SSHProtocolLink {

	pbytes, err := ioutil.ReadFile(pkeyFile)

	if err != nil {
		panic(fmt.Sprintf("ReadError %v \nFailed to load private key file: %s", err, pkeyFile))
	}

	private, err := ssh.ParsePrivateKey(pbytes)

	if err != nil {
		log.Println(fmt.Sprintf("ParseError:(%s):", pkeyFile), err)
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

	nsh := &SSHProtocolLink{
		NewProtocolLink(desc),
		config,
		nil,
		flux.NewAction(),
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

	nsh := &SSHProtocolLink{
		NewProtocolLink(desc),
		config,
		nil,
		flux.NewAction(),
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

	h.conn = client

	if err != nil {
		fmt.Println(fmt.Sprintf("client creation received error for: %s %v", h.Descriptor().Host(), err))
		// return err
	}

	return err
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
func (h *SSHProtocolLink) Command(cmd string) flux.ActionStackInterface {
	rq := h.Request("", nil)
	dn := rq.Done()
	ch := flux.NewActDepend(dn, 2)

	ch.Then(WhenSSHClientSession(func(s *SSHClientSession, next flux.ActionInterface) {
		s.Run(cmd)
		next.Fullfill(s)
	}))

	avs := flux.UnwrapActDependWrap(dn)
	avs.MixLast(0, ch)

	room := flux.NewActionStackBy(ch, rq.Error())

	return room
}

//Request is the base level method upon which all protocolink requests are handled
//for the sake of being a valid protcollink it implements the method signature
// (string,io.Reader) but for flexibilty string
func (h *SSHProtocolLink) Request(path string, body io.Reader) flux.ActionStackInterface {

	req := flux.NewAction()
	err := flux.NewAction()
	act := req.Chain(2)
	st := flux.NewActionStackBy(act, err)

	if h.conn == nil {
		st.Complete(ErrorNoConnection)
		return st
	}

	act.OverrideBefore(1, func(b interface{}, next flux.ActionInterface) {
		se, ok := b.(*SSHClientSession)

		if !ok {
			return
		}

		se.Close()
		next.Fullfill(b)
	})

	session, erro := h.conn.NewSession()

	if erro != nil {
		st.Complete(erro)
	} else {
		st.Complete(NewSSHClientSession(session, body))
	}

	return st

}
