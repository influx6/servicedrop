package servicedrop

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	// "code.google.com/p/go.crypto/ssh"

	"github.com/influx6/flux"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type (

	//UDPPacket represents a udp packet information
	UDPPacket struct {
		Path    string       `json:"path"`
		Service string       `json:"service"`
		UUID    string       `json:"uuid"`
		Data    []byte       `json:"data"`
		Address *net.UDPAddr `json:"address"`
	}

	//HTTPRequestPacket represent a standard http server request and responsewriter
	HTTPRequestPacket struct {
		Req          *http.Request
		Res          http.ResponseWriter
		Body         []byte
		RequestError error
		RequestType  string
	}

	//HTTPPacket represents a resolved http request contain the body,req and res object
	//with the bodyError and ResponseError returned when making a http request
	HTTPPacket struct {
		Res           *http.Response
		Req           *http.Request
		Body          []byte
		ResponseError error
		BodyReadError error
	}

	//SSHPacket represents a base level packet sent through ssh links and servers
	SSHPacket struct {
		Cmd string
	}

	//SSHClientSession is used to represent a current working ssh session
	SSHClientSession struct {
		*ssh.Session
		Read  io.Reader
		Write io.Writer
		Erros io.Writer
	}

	//SessionManagerInterface represent the member function rules for a session manager
	SessionManagerInterface interface {
		AddSession(net.Addr, Session)
		DestroySession(net.Addr)
		GetSession(net.Addr) (Session, error)
	}

	//SessionManager is used to managed session data for any service using the
	//net.Addr as a key
	SessionManager struct {
		sessions *flux.SecureMap
	}

	//Session is a map that can contain the data needed for use
	Session interface {
		UUID() string
		Addr() string
		User() string
		Pass() []byte
		Start() time.Time
		End() time.Time
		Reader() io.ReadCloser
		Incoming() flux.StreamInterface
		Outgoing() flux.StreamInterface
		Close()
	}
)

var (
	//ErrorNotFind stands for errors when value not find
	ErrorNotFind = errors.New("NotFound!")
	//ErrorBadRequestType stands for errors when the interface{} recieved can not
	//be type asserted as a *http.Request object
	ErrorBadRequestType = errors.New("type is not a *http.Request")
	//ErrorBadHTTPPacketType stands for errors when the interface{} received is not a
	//bad request type
	ErrorBadHTTPPacketType = errors.New("type is not a HTTPPacket")
	//ErrorNoConnection describe when a link connection does not exists"
	ErrorNoConnection = errors.New("NoConnection")
	//ExcessSlash is a regexp handling more than one /
	ExcessSlash = regexp.MustCompile(`/+`)
	//EndSlash is a regexp for ending slashes /
	EndSlash = regexp.MustCompile(`/+$`)
)

//ConnectionProc is the type used by the proxy-ssh-protocol for behaviour
// type ConnectionProc func()

//KeyAuthenticationCallback is the type for the ssh-server key-callback function
type KeyAuthenticationCallback func(ProtocolInterface, ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error)

//PasswordAuthenticationCallback is the type for the ssh-server password-callback function
type PasswordAuthenticationCallback func(ProtocolInterface, ssh.ConnMetadata, []byte) (*ssh.Permissions, error)

//KeyAuth represents a PublicCallback type
type KeyAuth func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error)

//PassAuth represents a PasswordCallback type
type PassAuth func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error)

//PasswordAuthenticationWrap wraps a PasswordAuthenticationCallback for use
func PasswordAuthenticationWrap(auth PasswordAuthenticationCallback, p ProtocolInterface) PassAuth {
	return func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		return auth(p, meta, pass)
	}
}

//KeyAuthenticationWrap wraps a KeyAuthenticationCallback for use
func KeyAuthenticationWrap(auth KeyAuthenticationCallback, p ProtocolInterface) KeyAuth {
	return func(meta ssh.ConnMetadata, pass ssh.PublicKey) (*ssh.Permissions, error) {
		return auth(p, meta, pass)
	}
}

//NewSessionManager allows management of sessions(ambiguous up to you to define that)
func NewSessionManager() *SessionManager {
	return &SessionManager{flux.NewSecureMap()}
}

//GetSession retrieves a session with the net.Addr
func (s *SessionManager) GetSession(addr net.Addr) (Session, error) {
	sm, ok := s.sessions.Get(addr).(Session)

	if ok {
		return sm, nil
	}

	return sm, ErrorNotFind
}

//AddSession adds a new settion with the address
func (s *SessionManager) AddSession(addr net.Addr, sm Session) {
	s.sessions.Set(addr, sm)
}

//DestroySession deletes a session and its content from the map
func (s *SessionManager) DestroySession(addr net.Addr) {
	sh, err := s.GetSession(addr)
	if err != nil {
		return
	}
	defer sh.Close()
	s.sessions.Remove(addr)
}

//NewSSHClientSession creates a new ssh session instance
func NewSSHClientSession(s *ssh.Session, in io.Reader) *SSHClientSession {
	out := new(bytes.Buffer)
	err := new(bytes.Buffer)
	s.Stdin = in
	s.Stdout = out
	s.Stderr = err
	return &SSHClientSession{s, in, out, err}
}

//Sanitize cleans a text for secure uses
func Sanitize(s string) string {
	s = strings.Replace(s, "<", "&lt;", -1)
	s = strings.Replace(s, ">", "&gt;", -1)
	s = strings.Replace(s, "\r", "", -1)
	s = strings.Replace(s, "\n", "<br/>", -1)
	s = strings.Replace(s, "'", "\\'", -1)
	s = strings.Replace(s, "\b", "<backspace>", -1)
	return s
}

//NewUDPPacket creates a new udp packet
func NewUDPPacket(path, service, uuid string, data []byte, addr *net.UDPAddr) *UDPPacket {
	return &UDPPacket{
		path,
		service,
		uuid,
		data,
		addr,
	}
}

//UDPPacketFrom creates a new udp packet from a previous one with only the data
//and addr changed
func UDPPacketFrom(u *UDPPacket, data []byte, addr *net.UDPAddr) *UDPPacket {
	return NewUDPPacket(u.Path, u.Service, u.UUID, data, addr)
}

//NewHTTPPacket returns a new http packet
func NewHTTPPacket(res *http.Response, req *http.Request, e error) *HTTPPacket {
	var bo []byte
	var err error

	if res != nil && res.Body != nil {
		defer res.Body.Close()
		bo, err = ioutil.ReadAll(res.Body)
	}

	return &HTTPPacket{
		res,
		req,
		bo,
		e,
		err,
	}
}

//FluxCallback type provides a type of the generic function flux caller
type FluxCallback func(interface{}, flux.ActionInterface)

//WhenHTTPRequest returns a function that wraps an input function checking if its
//indeed a http.Request object
func WhenHTTPRequest(fx func(*http.Request, flux.ActionInterface)) FluxCallback {
	return func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*http.Request)

		if !ok {
			// next.Fullfill(ErrorBadRequestType)
			return
		}

		fx(rq, next)
	}
}

//WhenHTTPPacket returns a function that wraps an input function and check the
//return function parameters if its a HTTPPacket type
func WhenHTTPPacket(fx func(*HTTPPacket, flux.ActionInterface)) FluxCallback {
	return func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*HTTPPacket)

		if !ok {
			// next.Fullfill(ErrorBadHTTPPacketType)
			return
		}

		fx(rq, next)
	}
}

//WhenSSHClientSession returns a function that wraps an input function and check the
//return function parameters if its a SSHSession type
func WhenSSHClientSession(fx func(*SSHClientSession, flux.ActionInterface)) FluxCallback {
	return func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*SSHClientSession)

		if !ok {
			// next.Fullfill(ErrorBadHTTPPacketType)
			return
		}

		fx(rq, next)
	}
}

//WhenFTPClient returns a function that wraps an input function and check the
//return function parameters if its a sfs.Client type
func WhenFTPClient(fx func(*sftp.Client, flux.ActionInterface)) FluxCallback {
	return func(b interface{}, next flux.ActionInterface) {
		rq, ok := b.(*sftp.Client)

		if !ok {
			return
		}

		fx(rq, next)
	}
}

//CollectHTTPBody takes a requests and retrieves the body from the into a gridpacket object
func CollectHTTPBody(r *http.Request, rw http.ResponseWriter) *HTTPRequestPacket {
	content, ok := r.Header["Content-Type"]
	muxcontent := strings.Join(content, ";")
	wind := strings.Index(muxcontent, "application/x-www-form-urlencode")
	mind := strings.Index(muxcontent, "multipart/form-data")

	var reqtype string
	var buff []byte
	var rerr error

	if ok {

		jsn := strings.Index(muxcontent, "application/json")

		if ok {

			if jsn != -1 {
				reqtype = "json"
			}

			if wind != -1 {
				if err := r.ParseForm(); err != nil {
					log.Println("Request Read Form Error", err)
					rerr = err
				} else {
					reqtype = "form"
				}
			}

			if mind != -1 {
				if err := r.ParseMultipartForm(32 << 20); err != nil {
					log.Println("Request Read MultipartForm Error", err)
					rerr = err
				} else {
					reqtype = "multipart"
				}
			}

			if mind == -1 && wind == -1 && r.Body != nil {

				buff = make([]byte, r.ContentLength)
				_, err := r.Body.Read(buff)

				if reqtype == "" {
					reqtype = "binary"
				}

				if err != nil {
					if err != io.EOF {
						log.Println("Request Read Body Error", err)
						rerr = err
					}
				}
			}
		}

	}

	return &HTTPRequestPacket{
		r,
		rw,
		buff,
		rerr,
		reqtype,
	}
}
