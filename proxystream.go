package servicedrop

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/influx6/flux"
)

type (

	//Notifier provides a nice type for a channel for struct{}
	Notifier chan struct{}
	//NotifierError provides a nice type for error chan
	NotifierError chan error

	// ProxyStream provides a basic level streaming connections
	ProxyStream struct {
		closed    Notifier
		clientend Notifier
		serverend Notifier
		errorend  NotifierError
		do        *sync.Once
		incoming  flux.StackStreamers
		outgoing  flux.StackStreamers
	}

	//ProxyStreams defines the streaming api member functions for streams
	ProxyStreams interface {
		Close() error
		In() flux.StackStreamers
		Out() flux.StackStreamers
		DestNotify() Notifier
		SrcNotify() Notifier
		ErrNotify() NotifierError
		CloseNotify() Notifier
		//implement in lower-class/subcomposition of stream
		DoBroker(io.WriteCloser, io.ReadCloser, Notifier)
		SrcReader() io.ReadCloser
		SrcWriter() io.WriteCloser
		DestReader() io.ReadCloser
		DestWriter() io.WriteCloser
		CloseDest() error
		CloseSrc() error
		// BufferedSrc() *bufio.ReadWriter
		// BufferedDest() *bufio.ReadWriter
		handleOperation()
	}

	//TCPProxyStream handles bare tcp stream proxying
	TCPProxyStream struct {
		*ProxyStream
		dest net.Conn
		src  net.Conn
	}

	//HTTPProxyStream provides http based proxying
	HTTPProxyStream struct {
		*TCPProxyStream
	}

	//HTTPSProxyStream provides https based proxying
	HTTPSProxyStream struct {
		*TCPProxyStream
	}

	//Nopwriter provides a writer with a Close member func
	Nopwriter struct {
		io.Writer
	}
)

//reportEror helper to just log out err from a error channel
func reportError(n NotifierError) {
	for ex := range n {
		log.Printf("Recieved Error %+s", ex.Error())
	}
}

//Close returns nil
func (n *Nopwriter) Close() error {
	return nil
}

//NopWriter returns a new nopwriter instance
func NopWriter(w io.Writer) *Nopwriter {
	return &Nopwriter{w}
}

//NewProxyStream returns a new proxy streamer
func NewProxyStream(in, out flux.StackStreamers) (p *ProxyStream) {

	if in == nil {
		in = flux.NewIdentityStream()
	}

	if out == nil {
		out = flux.NewIdentityStream()
	}

	p = &ProxyStream{
		closed:    make(Notifier),
		serverend: make(Notifier, 1),
		clientend: make(Notifier, 1),
		errorend:  make(NotifierError),
		do:        new(sync.Once),
		incoming:  in,
		outgoing:  out,
	}
	return
}

//DestNotify treturns the internal destination channel
func (p *ProxyStream) DestNotify() Notifier {
	return p.serverend
}

//SrcNotify treturns the internal destination channel
func (p *ProxyStream) SrcNotify() Notifier {
	return p.clientend
}

//ErrNotify treturns the internal destination channel
func (p *ProxyStream) ErrNotify() NotifierError {
	return p.errorend
}

//CloseNotify treturns the internal destination channel
func (p *ProxyStream) CloseNotify() Notifier {
	return p.closed
}

//Out treturns the internal out stream of this proxy
func (p *ProxyStream) Out() flux.StackStreamers {
	return p.outgoing
}

//In returns the internal in stream of this proxy
func (p *ProxyStream) In() flux.StackStreamers {
	return p.incoming
}

//handleOperation can be used to perform some work
func (p *ProxyStream) handleOperation() {
	go reportError(p.ErrNotify())

	select {
	case <-p.CloseNotify():
		log.Println("Closing destination first streams!")
		ex := p.CloseDest()
		if ex != nil {
			go func() { p.ErrNotify() <- ex }()
		}
	case <-p.SrcNotify():
		//close the server and notifier serverend
		ex := p.CloseDest()
		if ex != nil {
			go func() { p.ErrNotify() <- ex }()
		}
	case <-p.DestNotify():
		//close the client and notifier clientend
		ex := p.CloseSrc()
		if ex != nil {
			go func() { p.ErrNotify() <- ex }()
		}
	}
}

//CloseDest closes the destination
func (p *ProxyStream) CloseDest() error {
	return nil
}

//CloseSrc closes the destination
func (p *ProxyStream) CloseSrc() error {
	return nil
}

//DestReader returns a buffered ReadWriter for the source
func (p *ProxyStream) DestReader() io.ReadCloser {
	return nil
}

//DestWriter returns a buffered ReadWriter for the dest
func (p *ProxyStream) DestWriter() io.WriteCloser {
	return nil
}

//SrcWriter returns a buffered ReadWriter for the source
func (p *ProxyStream) SrcWriter() io.WriteCloser {
	return nil
}

//SrcReader returns a buffered ReadWriter for the dest
func (p *ProxyStream) SrcReader() io.ReadCloser {
	return nil
}

//DoBroker empty broker implementation
func (p *ProxyStream) DoBroker(d io.Writer, s io.Reader, r Notifier) {
}

//Close stops the streaming
func (p *ProxyStream) Close() error {
	p.do.Do(func() {
		close(p.closed)
	})
	return nil
}

//NewTCPProxyStream provides a tcp proxy streamer
func NewTCPProxyStream(src net.Conn, dest net.Conn, in, out flux.StackStreamers) *TCPProxyStream {
	return &TCPProxyStream{
		ProxyStream: NewProxyStream(in, out),
		src:         src,
		dest:        dest,
	}
}

//TCPStream provides a tcp proxy streamer
func TCPStream(src net.Conn, dest net.Conn, in, out flux.StackStreamers) ProxyStreams {
	ts := NewTCPProxyStream(src, dest, in, out)
	go ts.handleProcess()
	return ts
}

//CloseDest closes the destination
func (p *TCPProxyStream) CloseDest() error {
	p.DestNotify() <- struct{}{}
	return p.dest.Close()
}

//handleProcess process handles the operation of the streams
func (p *TCPProxyStream) handleProcess() {
	rdest := p.DestReader()
	wdest := p.DestWriter()

	rsrc := p.SrcReader()
	wsrc := p.SrcWriter()

	destwriter := NopWriter(io.MultiWriter(wdest, p.Out()))
	srcwriter := NopWriter(io.MultiWriter(wsrc, p.In()))

	go p.DoBroker(destwriter, rsrc, p.SrcNotify())
	go p.DoBroker(srcwriter, rdest, p.DestNotify())
	go p.handleOperation()
}

//CloseSrc closes the destination
func (p *TCPProxyStream) CloseSrc() error {
	p.SrcNotify() <- struct{}{}
	return p.src.Close()
}

//DestReader returns a buffered ReadWriter for the source
func (p *TCPProxyStream) DestReader() io.ReadCloser {
	return ioutil.NopCloser(bufio.NewReader(p.dest))
}

//DestWriter returns a buffered ReadWriter for the dest
func (p *TCPProxyStream) DestWriter() io.WriteCloser {
	return io.WriteCloser(p.dest)
}

//SrcWriter returns a buffered ReadWriter for the source
func (p *TCPProxyStream) SrcWriter() io.WriteCloser {
	return io.WriteCloser(p.src)
}

//SrcReader returns a buffered ReadWriter for the dest
func (p *TCPProxyStream) SrcReader() io.ReadCloser {
	return ioutil.NopCloser(bufio.NewReader(p.src))
}

//DoBroker provides the internal opeations of the streamer
func (p *TCPProxyStream) DoBroker(dest io.WriteCloser, src io.ReadCloser, end Notifier) {
	_, ex := io.Copy(dest, src)

	if ex != nil {
		go func() { p.errorend <- ex }()
	}

	end <- struct{}{}
}

//handleProcess process handles the operation of the streams
func (p *HTTPSProxyStream) handleProcess() {
	rdest := p.DestReader()
	wdest := p.DestWriter()

	rsrc := p.SrcReader()
	wsrc := p.SrcWriter()

	destwriter := NopWriter(io.MultiWriter(wdest, p.Out()))
	srcwriter := NopWriter(io.MultiWriter(wsrc, p.In()))

	defer p.DoBroker(destwriter, rsrc, p.SrcNotify())
	defer p.DoBroker(srcwriter, rdest, p.DestNotify())
	defer p.handleOperation()

	tlscon, ok := p.src.(*tls.Conn)

	if !ok {
		log.Println("HTTPSProxy: Could not type assert tls.Conn!")
		return
	}

	log.Println("HTTPSProxy: We received tls assertion success!")
	err := tlscon.HandShake()

	if err != nil {
		log.Printf("HTTPSProxy: HandShake failed (%+s)!", err)
		return
	}

	log.Printf("HTTPSProxy: HandShake Successful!")
	state := tlscon.ConnectionState()

	log.Println("HTTPSProxy: Reaching for ServerKeys")

	var clientcert *x509.Certificate

	for _, v := range state.PeerCertificates {
		log.Printf("%#+v", v.Subject.CommonName)
		log.Printf("%#+v", v.Subject.SerialNumber)
		clientcert = v
	}

	reader := bufio.NewReader(tls)
	req, err := http.ReadRequest(reader)

	if err != nil {
		log.Printf("HTTPSProxy: ReadRequest failed (%+s)!", err)
		return
	}

	if req == nil {
		log.Printf("HTTPSProxy: Received No Request (%+s)!", err)
		return
	}

	req.Write(destwriter)

	desttls, ok := p.dest.(*tls.Conn)

	if !ok {
		log.Printf("HTTPSProxy: Incorrect tls.Conn assert for destination!")
		return
	}

	resread := bufio.NewReader(desttls)
	res, err := http.ReadResponse(resread)

	if err != nil {
		log.Printf("HTTPSProxy: ReadResponse failed (%+s)!", err)
		return
	}

	if res == nil {
		log.Printf("HTTPSProxy: NoResponse received!")
		return
	}

	res.Write(srcwriter)

	//remove token cookie
	log.Println("HTTPSProxy: Finalized Proxy Request")
}

//DoBroker provides the internal opeations of the streamer
func (p *HTTPSProxyStream) DoBroker(dest io.WriteCloser, src io.ReadCloser, end Notifier) {
	end <- struct{}{}
}

//DoBroker provides the internal opeations of the streamer
func (p *HTTPProxyStream) DoBroker(dest io.WriteCloser, src io.ReadCloser, end Notifier) {
	end <- struct{}{}
}

//handleProcess process handles the operation of the streams
func (p *HTTPProxyStream) handleProcess() {
	rdest := p.DestReader()
	wdest := p.DestWriter()

	rsrc := p.SrcReader()
	wsrc := p.SrcWriter()

	destwriter := NopWriter(io.MultiWriter(wdest, p.Out()))
	srcwriter := NopWriter(io.MultiWriter(wsrc, p.In()))

	defer p.DoBroker(destwriter, rsrc, p.SrcNotify())
	defer p.DoBroker(srcwriter, rdest, p.DestNotify())
	defer p.handleOperation()

	// tls, ok := p.src.(*http.Conn)
	//
	// if !ok {
	// 	log.Println("HTTPSProxy: Could not type assert http.Conn!")
	// 	return
	// }

	// log.Println("HTTPSProxy: We received tls assertion success!")
	// err := tlscon.HandShake()
	//
	// if err != nil {
	// 	log.Printf("HTTPSProxy: HandShake failed (%+s)!", err)
	// 	return
	// }
	//
	// log.Printf("HTTPSProxy: HandShake Successful!")
	// state := tlscon.ConnectionState()
	//
	// log.Println("HTTPSProxy: Reaching for ServerKeys")
	//
	// var clientcert *x509.Certificate = nil
	//
	// for _, v := range state.PeerCertificates {
	// 	log.Printf("%#+v", v.Subject.CommonName)
	// 	log.Printf("%#+v", v.Subject.SerialNumber)
	// 	clientcert = v
	// }

	reader := bufio.NewReader(p.src)
	req, err := http.ReadRequest(reader)

	if err != nil {
		log.Printf("HTTPSProxy: ReadRequest failed (%+s)!", err)
		return
	}

	if req == nil {
		log.Printf("HTTPSProxy: Received No Request (%+s)!", err)
		return
	}

	req.Write(destwriter)

	// desttls, ok := p.dest.(net.Conn)
	//
	// if !ok {
	// 	log.Printf("HTTPSProxy: Incorrect tls.Conn assert for destination!")
	// 	return
	// }

	resread := bufio.NewReader(p.dest)
	res, err := http.ReadResponse(resread)

	if err != nil {
		log.Printf("HTTPSProxy: ReadResponse failed (%+s)!", err)
		return
	}

	if res == nil {
		log.Printf("HTTPSProxy: NoResponse received!")
		return
	}

	res.Write(srcwriter)

	//remove token cookie
	log.Println("HTTPSProxy: Finalized Proxy Request")
}
