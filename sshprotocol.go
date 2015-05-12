package servicedrop

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	// "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type (
	//SSHProtocol handles the server connection of the ssh protcol
	SSHProtocol struct {
		*Protocol
		conf    *ssh.ServerConfig
		servers []*ssh.ServerConn
		// sessions map[net.Addr]*Session
	}
)

//RSASSHProtocol creates a ssh-server that handles ssh-connections
func RSASSHProtocol(service, addr string, port int, rsaFile string, auth KeyAuthenticationCallback) *SSHProtocol {
	pbytes, err := ioutil.ReadFile(rsaFile)

	if err != nil {
		panic(fmt.Sprintf("ReadError %v \nFailed to load private key file: %s", err, rsaFile))
	}

	private, err := ssh.ParsePrivateKey(pbytes)

	if err != nil {
		panic(fmt.Sprintf("ParseError:(%s): \n %s -> %v", rsaFile, "Failed to parse private key!", err))
	}

	desc := NewDescriptor("ssh", service, addr, port, "0", "ssh")

	conf := &ssh.ServerConfig{
		PublicKeyCallback: auth,
	}

	conf.AddHostKey(private)

	return &SSHProtocol{
		NewProtocol(desc),
		conf,
		make([]*ssh.ServerConn, 0),
	}
}

//PasswordSSHProtocol creates a ssh-server that handles ssh-connections
func PasswordSSHProtocol(service, addr string, port int, rsaFile string, auth PasswordAuthenticationCallback) *SSHProtocol {
	pbytes, err := ioutil.ReadFile(rsaFile)

	if err != nil {
		panic(fmt.Sprintf("ReadError %v \nFailed to load private key file: %s", err, rsaFile))
	}

	private, err := ssh.ParsePrivateKey(pbytes)

	if err != nil {
		panic(fmt.Sprintf("ParseError:(%s): \n %s -> %v", rsaFile, "Failed to parse private key!", err))
	}

	desc := NewDescriptor("ssh", service, addr, port, "0", "ssh")

	conf := &ssh.ServerConfig{
		PasswordCallback: auth,
	}

	conf.AddHostKey(private)

	return &SSHProtocol{
		NewProtocol(desc),
		conf,
		make([]*ssh.ServerConn, 0),
	}
}

//Dial creates and connects the ssh server with the given details from the ProtocolDescription
func (s *SSHProtocol) Dial() error {

	tcpcon, err := net.Listen("tcp", s.Descriptor().Host())

	if err != nil {
		panic(fmt.Sprintf("Unable to create tcp connection for ssh-server: -> %v", err))
		// return err
	}

	for {
		con, err := tcpcon.Accept()

		if err != nil {
			log.Println(fmt.Sprintf("Connection Accept Error: -> %v", err))
			continue
		}

		conn, schan, req, err := ssh.NewServerConn(con, s.conf)

		if err != nil {
			log.Println(fmt.Sprintf("Unable to accept connection: -> %v", err))
			continue
		}

		log.Printf("Received new connection %v %v", conn.RemoteAddr(), conn.ClientVersion())

		s.servers = append(s.servers, conn)

		go s.handleRequest(req)
		go s.handleChannel(schan)

	}
}

func (s *SSHProtocol) handleChannel(sc <-chan ssh.NewChannel) {
	for curChan := range sc {
		switch curChan.ChannelType() {
		case "session":
		case "":
		}
	}
}

//handleRequest simple throws away out-of-bounds request
func (s *SSHProtocol) handleRequest(req <-chan *ssh.Request) {
	ssh.DiscardRequests(req)
}
