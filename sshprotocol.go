package servicedrop

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
	// "github.com/pkg/sftp"
	"github.com/influx6/flux"
	"github.com/kr/pty"
	"golang.org/x/crypto/ssh"
)

type (
	//SSHProtocol handles the server connection of the ssh protcol
	SSHProtocol struct {
		*Protocol
		NetworkOpen  flux.Pipe
		NetworkClose flux.Pipe
		conf         *ssh.ServerConfig
		servers      []*ssh.ServerConn
		tcpCon       net.Listener
	}

	//SSHProxyProtocol handles the sshprotcol created and proxies all its connection
	SSHProxyProtocol struct {
		*SSHProtocol
	}

	//WinDim stores the windows dimension for a terminal
	WinDim struct {
		Height uint16
		Width  uint16
		x      uint16
		y      uint16
	}

	//Pty represents the pseudo-shell created for the connection
	Pty struct {
		Tty *os.File
		Pfd *os.File
	}

	//ChannelPayload defines a payload containing the channel and request of the server
	ChannelPayload struct {
		Chan ssh.Channel
		Req  *ssh.Request
		Pty  *Pty
		Do   *sync.Once
	}

	//ChannelNetwork contains specific data which is used to pass data into other
	//operations used on the ssh-protocol,this is sent into the sshProtocol Network socket
	ChannelNetwork struct {
		Conn          ssh.ConnMetadata
		Chan          ssh.Channel
		MasterChan    ssh.NewChannel
		ChanCloser    chan struct{}
		MaseterCloser chan struct{}
	}

	//ClientManager provides a function that returns a ssh.Client
	ClientManager func(*ChannelNetwork) (*ssh.Client, error)

	//ChannelMaker provides a function that returns a reader for a client
	ChannelMaker func(*ChannelNetwork, ssh.Channel) (io.ReadCloser, error)
)

var (
	shell = os.Getenv("SHELL")
)

//ClientProxySSHProtocol builds on top of the base proxy
func ClientProxySSHProtocol(s *SSHProtocol, cm ClientManager, cmk ChannelMaker) (base *SSHProxyProtocol) {
	base = BaseProxySSHProtocol(s)

	base.NetworkOpen.Subscribe(func(b interface{}, s *flux.Sub) {
		nc, ok := b.(*ChannelNetwork)

		if !ok {
			return
		}

		client, err := cm(nc)

		if err != nil {
			log.Printf("Unable to find session client for (%+v)", nc.Conn.RemoteAddr())
			return
		}

		defer client.Close()

		rcChannel, rcReq, err := client.OpenChannel(nc.MasterChan.ChannelType(), nc.MasterChan.ExtraData())

		if err != nil {
			log.Printf("Error creating ClientChannel for %+v %+v", nc.MasterChan.ChannelType(), err)
			return
		}

		go func() {
		clientloop:
			for {
				select {
				case <-nc.ChanCloser:
					break clientloop
				case <-nc.MaseterCloser:
					break clientloop
				case rq := <-rcReq:
					log.Println("creating RequestChannel for:", rq.Type)

					do, err := rcChannel.SendRequest(rq.Type, rq.WantReply, rq.Payload)

					if err != nil {
						log.Printf("Request proxy failed on: (%s) (%+v) with error (%+v)", nc.Conn.RemoteAddr(), rq.Type, err)
					}

					if rq.WantReply {
						rq.Reply(do, nil)
					}
				default:
					//logit
				}
			}
		}()

		var wrapMaster io.ReadCloser = nc.Chan
		var wrapSlave io.ReadCloser = rcChannel

		if cmk != nil {
			rw, err := cmk(nc, rcChannel)

			if err != nil {
				log.Println("Error creating custom reader for channel", err)
			} else {
				wrapSlave = rw
			}
		}

		go io.Copy(rcChannel, wrapMaster)
		go io.Copy(nc.Chan, wrapSlave)

		defer wrapMaster.Close()
		defer wrapSlave.Close()

		base.NetworkClose.Emit(nc)

		return
	})

	return
}

//BaseProxySSHProtocol returns a ssh protocol to wrap over an sshprotocol
func BaseProxySSHProtocol(s *SSHProtocol) *SSHProxyProtocol {
	proxy := &SSHProxyProtocol{s}
	AddRedirectBehaviour(proxy.SSHProtocol)
	return proxy
}

//RSASSHProtocol creates a ssh-server that handles ssh-connections
func RSASSHProtocol(rc *RouteConfig, service, addr string, port int, rsaFile string, auth KeyAuth) *SSHProtocol {
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

	sd := &SSHProtocol{
		BaseProtocol(desc, rc),
		flux.PushSocket(0),
		flux.PushSocket(0),
		conf,
		make([]*ssh.ServerConn, 0),
		nil,
	}

	setupRoutes(sd)

	return sd
}

//PasswordSSHProtocol creates a ssh-server that handles ssh-connections
func PasswordSSHProtocol(rc *RouteConfig, service, addr string, port int, rsaFile string, auth PassAuth) *SSHProtocol {
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

	sd := &SSHProtocol{
		BaseProtocol(desc, rc),
		flux.PushSocket(0),
		flux.PushSocket(0),
		conf,
		make([]*ssh.ServerConn, 0),
		nil,
	}

	setupRoutes(sd)

	// sd.Routes().Sub(func(r *Request, s *flux.Sub) {
	// 	log.Printf("req: %+v %+v %+v", r.Paths, r.Payload, r)
	//
	// })

	return sd
}

func setupRoutes(s *SSHProtocol) {
	s.Routes().New("session/exec")
	s.Routes().New("session/pty-req")
	s.Routes().New("session/env")
	s.Routes().New("session/shell")
	s.Routes().New("session/window-change")
}

//PtyRun assigns a psuedo terminal tty to the corresponding std.io set and returns an error indicating state
func PtyRun(c *exec.Cmd, tty *os.File) (err error) {
	defer tty.Close()
	c.Stdout = tty
	c.Stdin = tty
	c.Stderr = tty
	c.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}
	return c.Start()
}

//ParseDimension takes a byte and extracts the 32 int value for width and height
func ParseDimension(b []byte) (uint32, uint32) {
	w := binary.BigEndian.Uint32(b)
	h := binary.BigEndian.Uint32(b[4:])
	return w, h
}

//MurphWindow changes the windows size for a given terminal
func MurphWindow(fd uintptr, w, h uint32) {
	log.Printf("Setting Terminal: (%+v) to (%dx%d)", fd, w, h)
	ws := &WinDim{Width: uint16(w), Height: uint16(h)}
	syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}

//AddRefusalRouteBehaviour allows to add the default response/actions for client refusing behaviour per
//route
func AddRefusalRouteBehaviour(s *Route) {
	s.Sub(func(r *Request, s *flux.Sub) {

		payload, ok := r.Payload.(*PayloadRack)

		if !ok {
			return
		}

		payload.Release().When(func(d interface{}, _ flux.ActionInterface) {
			cpay, ok := d.(*ChannelPayload)

			if !ok {
				return
			}

			cpay.Do.Do(func() {
				cpay.Req.Reply(false, nil)
			})

		})

	})
}

//AddRefusalBehaviour sets a particular sshserver to refuse all requests
func AddRefusalBehaviour(s *SSHProtocol) {
	AddRefusalRouteBehaviour(s.Routes())
}

//AddRedirectRouteBehaviour allows to add the default response/actions for client redirection behaviour per
//route
func AddRedirectRouteBehaviour(s *Route) {
	s.Sub(func(r *Request, s *flux.Sub) {

		payload, ok := r.Payload.(*PayloadRack)

		if !ok {
			return
		}

		payload.Release().When(func(d interface{}, _ flux.ActionInterface) {
			cpay, ok := d.(*ChannelPayload)

			if !ok {
				return
			}

			ro, err := cpay.Chan.SendRequest(cpay.Req.Type, cpay.Req.WantReply, cpay.Req.Payload)

			if err != nil {
				log.Printf("Proxy request failed: %+v", err)
			}

			if cpay.Req.WantReply {
				cpay.Do.Do(func() {
					cpay.Req.Reply(ro, nil)
				})
			}

		})

	})
}

//AddRedirectBehaviour allows to add the default response/actions for client proxy
func AddRedirectBehaviour(s *SSHProtocol) {
	AddRedirectRouteBehaviour(s.Routes())
}

//AddPtyBehaviour allows to add the default response/actions for pty-request
func AddPtyBehaviour(s *SSHProtocol) {
	s.Routes().Child("session/pty-req").Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving pty-req request:", data.Paths)

		payload, ok := data.Payload.(*PayloadRack)

		if !ok {
			return
		}

		payload.Release().When(func(d interface{}, _ flux.ActionInterface) {
			cpay, ok := d.(*ChannelPayload)

			if !ok {
				return
			}

			if cpay.Pty != nil {
				cpay.Do.Do(func() {
					log.Println("Pty allowed!")
					termlen := cpay.Req.Payload[3]
					termEnv := string(cpay.Req.Payload[4 : termlen+4])
					w, h := ParseDimension(cpay.Req.Payload[termlen+4:])
					MurphWindow(cpay.Pty.Pfd.Fd(), w, h)
					log.Printf("Pty morhp for '%s'", termEnv)

					if cpay.Req.WantReply {
						cpay.Req.Reply(true, nil)
					}
				})
			}

		})
	})
}

//AddShellBehaviour allows to add the default response/actions for shell-request
func AddShellBehaviour(s *SSHProtocol) {
	if shell == "" {
		shell = "sh"
	}

	s.Routes().Child("session/shell").Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving request:", data.Paths)

		payload, ok := data.Payload.(*PayloadRack)

		if !ok {
			return
		}

		payload.Release().When(func(d interface{}, _ flux.ActionInterface) {
			cpay, ok := d.(*ChannelPayload)

			if !ok {
				return
			}

			if cpay.Pty != nil {
				cpay.Do.Do(func() {
					log.Println("Pty allowed!")

					cmd := exec.Command(shell)
					cmd.Env = []string{"TERM=xterm"}

					err := PtyRun(cmd, cpay.Pty.Tty)

					if err != nil {
						log.Printf("Error running shell command %+v", err)
					}

					teardown := new(sync.Once)
					closer := func() {
						cpay.Chan.Close()
						log.Printf("Closing session")
					}

					go func() {
						io.Copy(cpay.Chan, cpay.Pty.Pfd)
						teardown.Do(closer)
					}()

					go func() {
						io.Copy(cpay.Pty.Pfd, cpay.Chan)
						teardown.Do(closer)
					}()

					if cpay.Req.WantReply {
						//For now commands are not being supported but still up for discussion
						if len(cpay.Req.Payload) == 0 {
							cpay.Req.Reply(true, nil)
						} else {
							cpay.Req.Reply(false, nil)
						}
					}
				})
			}

		})
	})
}

//AddWindowChangeBehaviour allows to add the default response/actions for window-change behaviour
func AddWindowChangeBehaviour(s *SSHProtocol) {
	if shell == "" {
		shell = "sh"
	}

	s.Routes().Child("session/window-change").Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving request:", data.Paths)

		payload, ok := data.Payload.(*PayloadRack)

		if !ok {
			return
		}

		payload.Release().When(func(d interface{}, _ flux.ActionInterface) {
			cpay, ok := d.(*ChannelPayload)

			if !ok {
				return
			}

			if cpay.Pty != nil {
				cpay.Do.Do(func() {
					w, h := ParseDimension(cpay.Req.Payload)
					MurphWindow(cpay.Pty.Pfd.Fd(), w, h)
					if cpay.Req.WantReply {
						cpay.Req.Reply(true, nil)
					}
				})
			}

		})
	})
}

//AddExecBehaviour allows to add the default response/actions for exec-request
func AddExecBehaviour(s *SSHProtocol) {
	if shell == "" {
		shell = "sh"
	}

	s.Routes().Child("session/exec").Sub(func(data *Request, s *flux.Sub) {
		log.Println("receiving request:", data.Paths)

		payload, ok := data.Payload.(*PayloadRack)

		if !ok {
			return
		}

		payload.Release().When(func(d interface{}, _ flux.ActionInterface) {
			cpay, ok := d.(*ChannelPayload)

			if !ok {
				return
			}

			if cpay.Pty != nil {
				cpay.Do.Do(func() {
					log.Println("Pty Exec command allowed!")
					command := string(cpay.Req.Payload[4 : cpay.Req.Payload[3]+4])
					cmd := exec.Command(shell, []string{"-c", command}...)

					cmd.Stdout = cpay.Chan
					cmd.Stdin = cpay.Chan
					cmd.Stderr = cpay.Chan

					err := cmd.Start()

					if err != nil {
						log.Printf("could not start command (%s)", err)
						if cpay.Req.WantReply {
							cpay.Req.Reply(false, nil)
						}
						return
					}

					go func() {
						_, err := cmd.Process.Wait()

						if err != nil {
							log.Printf("failed to exit shell (%s)", err)
						}
						cpay.Chan.Close()
						log.Printf("session closed")
					}()

					if cpay.Req.WantReply {
						cpay.Req.Reply(true, nil)
					}
				})
			}

		})
	})
}

//Dial creates and connects the ssh server with the given details from the ProtocolDescription
func (s *SSHProtocol) Dial() error {

	tcpcon, err := net.Listen("tcp", s.Descriptor().Host())

	if err != nil {
		panic(fmt.Sprintf("Unable to create tcp connection for ssh-server: -> %v", err))
		// return err
	}

	s.tcpCon = tcpcon

	defer tcpcon.Close()
	defer func() {
		s.tcpCon = nil
	}()

	func() {
		go func() {
			<-s.ProtocolClosed
			// conn.Close()
			// con.Close()
			log.Println("killing")
			tcpcon.Close()
			// panic("killing all processes")
		}()

	loopmaker:
		for {
			con, err := tcpcon.Accept()

			if err != nil {
				log.Println(fmt.Sprintf("Connection Accept Error: -> %v", err))
				continue
			}

			conn, schan, req, err := ssh.NewServerConn(con, s.conf)

			if err != nil {
				log.Println(fmt.Sprintf("Unable to accept connection: -> %v", err))
				continue loopmaker
			}

			// defer conn.Close()

			s.servers = append(s.servers, conn)

			go s.HandleRequest(req, conn)
			go s.HandleChannel(schan, conn)

		}
	}()

	return err
}

//Drop ends the connection used by this service
func (s *SSHProtocol) Drop() error {
	close(s.ProtocolClosed)

	// go func() {
	// 	err := recover()
	//
	// 	if err != nil {
	// 		log.Printf("Process died due to (%+v)", err)
	// 	}
	// }()
	//
	// panic("close all connections")
	// if s.tcpCon != nil {
	// 	s.tcpCon.Close()
	// }
	//
	// for _, sv := range s.servers {
	// 	sv.Close()
	// }

	return nil
}

//HandleChannel manages the handling of a ConnectionChannel requests channels
func (s *SSHProtocol) HandleChannel(sc <-chan ssh.NewChannel, d *ssh.ServerConn) {
	closer := make(chan struct{})

	defer d.Close()
	defer close(closer)

	channelProc := func(curChan ssh.NewChannel) {
		stype := curChan.ChannelType()
		rw := s.Routes().Child(stype)

		if rw == nil {
			curChan.Reject(ssh.UnknownChannelType, "unknown not supported!")
			return
			// continue
		}

		log.Printf("Accepting connections for (%s)", stype)

		ch, reqs, err := curChan.Accept()

		s.NetworkOpen.Emit(&ChannelNetwork{d, ch, curChan, closer, s.ProtocolClosed})

		if err != nil {
			log.Println("Error accepting channel: ", err)
			return
			// continue
		}

		log.Printf("Creating pty terminal for (%s)", stype)

		fd, tty, err := pty.Open()

		log.Printf("Pty Successful? %+v ", err == nil)

		if err != nil {
			log.Printf("Unable to open pty (%+v):(%+v)", err, fd)
			return
			// continue
		}

		pterm := &Pty{tty, fd}

		go func(in <-chan *ssh.Request) {
		chanHandle:
			for greq := range in {
				reqtype := greq.Type

				if reqtype == "exit-status" {
					ch.Close()
					break chanHandle
				}

				path := fmt.Sprintf("%s/%s/%s", s.Descriptor().Service, stype, reqtype)
				s.Routes().Serve(path, &ChannelPayload{
					ch,
					greq,
					pterm,
					new(sync.Once),
				}, -1)
			}
		}(reqs)
	}

	func() {
	loopy:
		for {
			select {
			case <-s.ProtocolClosed:
				break loopy
			case dc := <-sc:
				if dc != nil {
					channelProc(dc)
				}
			default:
				//logit
			}
		}
	}()

}

//HandleRequest simple handless a ssh.ServerConn  out-of-bounds request
func (s *SSHProtocol) HandleRequest(req <-chan *ssh.Request, d *ssh.ServerConn) {
	_ = d
	ssh.DiscardRequests(req)
}
