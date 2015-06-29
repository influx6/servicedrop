package servicedrop

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime/debug"
	"sync"
	"syscall"
	"unsafe"
	// "github.com/pkg/sftp"
	"code.google.com/p/go-uuid/uuid"
	"github.com/honeycast/lxcontroller"
	"github.com/influx6/flux"
	"github.com/kr/pty"
	"golang.org/x/crypto/ssh"
)

type (
	//SSHProtocol handles the server connection of the ssh protcol
	SSHProtocol struct {
		*Protocol
		NetworkReaders   flux.Pipe
		NetworkChannels  flux.Pipe
		NetworkOutbounds flux.Pipe
		ServerConf       *ssh.ServerConfig
		TCPCon           net.Listener
		Before           *NetworkReflex
		After            *NetworkReflex
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

	//SSHSession defines a standard session contain information used by lxc servers
	SSHSession interface {
		Session
		Container() lxcontroller.Controllers
		Connection() *ssh.Client
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
		Conn             ssh.ConnMetadata
		MasterChan       ssh.Channel
		MasterNewChan    ssh.NewChannel
		ChanCloser       chan struct{}
		MaseterCloser    chan struct{}
		MasterReqChannel <-chan *ssh.Request
		Pty              *Pty
	}

	//ChannelReader is used to send the readers for using the channels
	ChannelReader struct {
		Session SSHSession
		Closer  chan struct{}
	}

	//ChannelPacket is used to handle off new channel requests from the ssh-server
	ChannelPacket struct {
		Conn *ssh.ServerConn
		Chan <-chan ssh.NewChannel
	}

	//RequestPacket is used to handle off new out of band channel requests from the ssh-server
	RequestPacket struct {
		Conn *ssh.ServerConn
		Reqs <-chan *ssh.Request
	}

	// //ClientManager provides a function that returns a ssh.Client
	// ClientManager func(*ChannelNetwork) (*ssh.Client, error)

	//ChannelMaker provides a function that returns a reader for a client
	ChannelMaker func(*ChannelNetwork, SSHSession, ssh.Channel) (io.ReadCloser, error)
)

var (
	shell = os.Getenv("SHELL")

	//ErrTimeout represents a timeout error
	ErrTimeout = errors.New("Timeout expired!")
)

//ClientProxySSHProtocol builds on top of the base proxy
func ClientProxySSHProtocol(s *SSHProtocol, cmk ChannelMaker) (base *SSHProxyProtocol) {
	base = BaseProxySSHProtocol(s)

	base.NetworkOpen.Subscribe(func(b interface{}, sub *flux.Sub) {
		nc, ok := b.(*ChannelNetwork)

		if !ok {
			return
		}

		log.Println("Network Open received network packet, prepare.....")
		si, err := base.Sessions().GetSession(nc.Conn.RemoteAddr())

		if err != nil {
			log.Printf("Unable to find session client for (%+v)", nc.Conn.RemoteAddr())
			return
		}

		session, ok := si.(SSHSession)

		if !ok {
			return
		}

		log.Printf("Session retrieved: (%+v) (%+v)", nc.Conn.RemoteAddr(), session.User())

		// defer session.Connection().Close()

		pid := uuid.New()
		session.UseType(pid)

		client := session.Connection()

		log.Printf("Session connection gained: %s, OpenChannel for %s", client.RemoteAddr(), nc.MasterNewChan.ChannelType())

		rcChannel, rcReq, err := client.OpenChannel(nc.MasterNewChan.ChannelType(), nc.MasterNewChan.ExtraData())

		if err != nil {
			log.Printf("Error creating ClientChannel for %+v %+v", nc.MasterNewChan.ChannelType(), err)
			return
		}

		log.Println("Success Creating Client proxy channel:", err)

		replyMaker := func(rq *ssh.Request, dest ssh.Channel) {
			do, err := dest.SendRequest(rq.Type, rq.WantReply, rq.Payload)

			if err != nil {
				log.Printf("Request proxy failed on: (%s) (%+v) with error (%+v)", nc.Conn.RemoteAddr(), rq.Type, err)
			}

			if rq.WantReply {
				rq.Reply(do, nil)
			}
		}

		go func() {
		clientloop:
			for {
				select {
				case <-nc.ChanCloser:
					break clientloop
				case <-nc.MaseterCloser:
					break clientloop
				case mrq, ok := <-nc.MasterReqChannel:
					if !ok {
						break clientloop
					}

					replyMaker(mrq, rcChannel)

					switch mrq.Type {
					case "exit-status":
						break clientloop
					}

				case rq, ok := <-rcReq:
					if !ok {
						break clientloop
					}

					replyMaker(rq, nc.MasterChan)

					switch rq.Type {
					case "exit-status":
						break clientloop
					}

				default:
					//logit
				}

			}

			log.Println("Closing Client and Master Channels for:", session.Addr())
			rcChannel.Close()
			nc.MasterChan.Close()
		}()

		log.Println("Creating channel readers and connection")

		//handle closing and state management of copying op
		copyCloser := new(sync.Once)
		copyState := make(chan struct{})
		loopCloser := make(chan struct{})

		log.Println("Creating channel and sync.Closer")

		copyCloseFn := func() {
			log.Println("Closing copying channel and client Operation operation for:", session.Addr())
			close(copyState)
			close(loopCloser)
		}

		log.Println("Setting up Writers")
		wrapMaster := io.ReadCloser(nc.MasterChan)
		wrapSlave := io.ReadCloser(rcChannel)

		if cmk != nil {
			rw, err := cmk(nc, session, rcChannel)

			if err != nil {
				log.Println("Error creating custom reader for channel", err)
			} else {
				wrapSlave = rw
			}
		}

		log.Printf("Connecting Sessions for (%s) At (%s) Packet Snifers", session.User(), session.Addr())

		outwriter := io.ReadWriteCloser(session.Outgoing())
		inwriter := io.ReadWriteCloser(session.Incoming())

		mwriter := io.MultiWriter(nc.MasterChan, outwriter)
		swriter := io.MultiWriter(rcChannel, inwriter)

		go func() {
			// io.Copy(rcChannel, wrapMaster)
			// io.Copy(session.Incoming(), wrapMaster)
			defer copyCloser.Do(copyCloseFn)
			io.Copy(swriter, wrapMaster)
		}()

		go func() {
			// io.Copy(nc.MasterChan, wrapSlave)
			// io.Copy(session.Outgoing(), wrapSlave)
			defer copyCloser.Do(copyCloseFn)
			io.Copy(mwriter, wrapSlave)
		}()

		go func() {
			defer base.NetworkClose.Emit(nc)
			<-copyState
			log.Println("Closing Incoming and Outgoing monitory Channels!")

			log.Println("Closing all Channels!")
			wrapMaster.Close()
			wrapSlave.Close()

			log.Println("closing session connection")
			session.Connection().Close()
			session.Close()
		}()

		return
	})

	return
}

//BaseRedirectProxySSHProtocol returns a ssh protocol to wrap over an sshprotocol using the AddRedirectBehaviour
func BaseRedirectProxySSHProtocol(s *SSHProtocol) *SSHProxyProtocol {
	proxy := &SSHProxyProtocol{s}
	AddRedirectBehaviour(proxy.SSHProtocol)
	return proxy
}

//BaseProxySSHProtocol returns a ssh protocol to wrap over an sshprotocol
func BaseProxySSHProtocol(s *SSHProtocol) *SSHProxyProtocol {
	proxy := &SSHProxyProtocol{s}
	proxy.SSHProtocol.NetworkChannels.ClearListeners()
	AddProxyChannelManager(proxy.SSHProtocol)
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
		flux.PushSocket(0),
		conf,
		nil,
		nil,
		nil,
	}

	setupServer(sd)

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
		flux.PushSocket(0),
		conf,
		nil,
		nil,
		nil,
	}

	setupServer(sd)

	return sd
}

func setupServer(s *SSHProtocol) {
	s.Routes().New("session/exec")
	s.Routes().New("session/pty-req")
	s.Routes().New("session/env")
	s.Routes().New("session/shell")
	s.Routes().New("session/window-change")
	AddStandardChannelManager(s)
	AddOutBoundRequestManager(s)
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
		log.Println("Receiving Request:", data.Paths)

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
		log.Println("Receiving request:", data.Paths)

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
						log.Printf("Could not start command (%s)", err)
						if cpay.Req.WantReply {
							cpay.Req.Reply(false, nil)
						}
						return
					}

					go func() {
						_, err := cmd.Process.Wait()

						if err != nil {
							log.Printf("Failed to exit shell (%s)", err)
						}
						cpay.Chan.Close()
						log.Printf("Session closed")
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

	log.Printf("Loading Connection Processes....")

	tcpcon, err := net.Listen("tcp", s.Descriptor().Host())

	if err != nil {
		panic(fmt.Sprintf("Unable to create tcp connection for ssh-server: -> %v", err))
		// return err
	}

	s.TCPCon = tcpcon

	defer tcpcon.Close()

	func() {
		go func() {
			<-s.ProtocolClosed
			defer func() {
				err := recover()

				if err != nil {
					log.Println("Recovered from Panic:", err, debug.Stack())
					return
				}

				return
			}()
			// conn.Close()
			// con.Close()
			log.Println("killing Process!")
			tcpcon.Close()
			panic("killing all processes")
		}()

	loopmaker:
		for {

			con, err := tcpcon.Accept()

			if s.Before != nil {
				err := s.Before.Do(con)
				if err != nil {
					log.Println(fmt.Sprintf("Connection Accept Error: -> %v", err))
					continue
				}
			}

			if err != nil {
				log.Println(fmt.Sprintf("Connection Accept Error: -> %v", err))
				continue
			}

			log.Printf("Accepting Connection Request from %s", con.RemoteAddr())

			conn, schan, req, err := ssh.NewServerConn(con, s.ServerConf)

			if err != nil {
				log.Println(fmt.Sprintf("TCPACCEPT-STAGE: Unable to accept connection: -> %v", err))
				continue loopmaker
			}

			if conn == nil || schan == nil || req == nil {
				log.Println("Nill pointer encountered in NewServerConn op")
				continue loopmaker
			}

			log.Println("New Connection created:", conn.RemoteAddr(), conn.LocalAddr())
			// defer conn.Close()

			// log.Println("Emitting New Channel")
			s.NetworkChannels.Emit(&ChannelPacket{conn, schan})
			// log.Println("Emitting Outof Bound")
			s.NetworkOutbounds.Emit(&RequestPacket{conn, req})

			//dont starve the cpu
			if s.After != nil {
				err := s.After.Do(con)
				if err != nil {
					log.Println(fmt.Sprintf("Connection: After Error: -> %v", err))
				}
			}

		}
	}()

	return err
}

//Drop ends the connection used by this service
func (s *SSHProtocol) Drop() error {
	close(s.ProtocolClosed)
	return nil
}

//AddStandardChannelManager manages the handling of a ConnectionChannel requests channels
func AddStandardChannelManager(s *SSHProtocol) {
	s.NetworkChannels.Subscribe(func(pack interface{}, sub *flux.Sub) {
		log.Println("----------------------------------------------------------")
		packet, ok := pack.(*ChannelPacket)

		if !ok {
			log.Printf("Received invalid ChannelPacket for ssh.NewChannel request: %+v %+v", packet, pack)
			return
		}

		go func() {
			sc := packet.Chan
			d := packet.Conn

			closer := make(chan struct{})

			defer d.Close()
			defer close(closer)
			// defer sub.Close()

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

				s.NetworkOpen.Emit(&ChannelNetwork{
					d,
					ch,
					curChan,
					closer,
					s.ProtocolClosed,
					reqs,
					pterm,
					// nil,
				})

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

			log.Printf("New Channel Listener is finished for LocalIp:%+v RemoteIp: %+v for User: %+v", d.LocalAddr(), d.RemoteAddr(), d.User())
			log.Println("----------------------------------------------------------")
		}()
	})
}

//AddProxyChannelManager manages the handling of a ConnectionChannel requests channels
func AddProxyChannelManager(s *SSHProtocol) {
	s.NetworkChannels.Subscribe(func(pack interface{}, sub *flux.Sub) {
		log.Println("----------------------------------------------------------")
		packet, ok := pack.(*ChannelPacket)

		if !ok {
			log.Printf("Received invalid ChannelPacket for ssh.NewChannel request: %+v %+v", packet, pack)
			return
		}

		go func() {
			// log.Printf("We got packet %+v", packet)
			sc := packet.Chan
			d := packet.Conn

			closer := make(chan struct{})

			defer d.Close()
			defer close(closer)
			// defer sub.Close()

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

				if err != nil {
					log.Println("TCPACCEPT: Error accepting channel: ", err)
					return
					// continue
				}

				s.NetworkOpen.Emit(&ChannelNetwork{
					d,
					ch,
					curChan,
					closer,
					s.ProtocolClosed,
					reqs,
					nil,
				})
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
							return
						}
						log.Println("Ending protocol action processes!")
					default:
						//logit
					}
				}
			}()

			log.Printf("New Channel Listener is finished for LocalIp:%+v RemoteIp: %+v for User: %+v", d.LocalAddr(), d.RemoteAddr(), d.User())
			log.Println("----------------------------------------------------------")
		}()

	})

}

//AddOutBoundRequestManager simple handless a ssh.ServerConn  out-of-bounds request
func AddOutBoundRequestManager(s *SSHProtocol) {
	s.NetworkOutbounds.Subscribe(func(pack interface{}, sub *flux.Sub) {
		packet, ok := pack.(*RequestPacket)

		if !ok {
			log.Printf("Received invalid RequestPacket for ssh.NewChannel request: %+v %+v", packet, pack)
			return
		}

		// log.Printf("Discarding Out-Of-Bands Requests: %v", packet)
		ssh.DiscardRequests(packet.Reqs)
	})
}
