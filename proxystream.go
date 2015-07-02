package servicedrop

import (
	"bufio"
	"io"
	"io/ioutil"
	"log"
	"net"
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
		DoBroker(io.Writer, io.Reader, Notifier)
		SrcReader() io.ReadCloser
		SrcWriter() io.WriteCloser
		DestReader() io.ReadCloser
		DestWriter() io.WriteCloser
		CloseDest() error
		CloseSrc() error
		// BufferedSrc() *bufio.ReadWriter
		// BufferedDest() *bufio.ReadWriter
		// handleOperation()
	}

	//TCPProxyStream handles bare tcp stream proxying
	TCPProxyStream struct {
		*ProxyStream
		dest net.Conn
		src  net.Conn
	}

	//HTTPProxyStream provides http based proxying
	HTTPProxyStream struct {
		*ProxyStream
	}

	//HTTPSProxyStream provides https based proxying
	HTTPSProxyStream struct {
		*ProxyStream
	}
)

//handle process handles the operation of the streams
func handleProcess(p ProxyStreams) {
	rdest := p.DestReader()
	wdest := p.DestWriter()

	rsrc := p.SrcReader()
	wsrc := p.SrcWriter()

	destwriter := io.MultiWriter(wdest, p.Out())
	srcwriter := io.MultiWriter(wsrc, p.In())

	go p.DoBroker(destwriter, rsrc, p.SrcNotify())
	go p.DoBroker(srcwriter, rdest, p.DestNotify())

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

//reportEror helper to just log out err from a error channel
func reportError(n NotifierError) {
	for ex := range n {
		log.Printf("Recieved Error %+s", ex.Error())
	}
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

//TCPStream provides a tcp proxy streamer
func TCPStream(src net.Conn, dest net.Conn, in, out flux.StackStreamers) ProxyStreams {
	ts := &TCPProxyStream{
		ProxyStream: NewProxyStream(in, out),
		src:         src,
		dest:        dest,
	}
	go handleProcess(ts)
	return ts
}

//CloseDest closes the destination
func (p *TCPProxyStream) CloseDest() error {
	p.DestNotify() <- struct{}{}
	return p.dest.Close()
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
func (p *TCPProxyStream) DoBroker(dest io.Writer, src io.Reader, end Notifier) {
	_, ex := io.Copy(dest, src)

	if ex != nil {
		go func() { p.errorend <- ex }()
	}

	end <- struct{}{}
}

//DoBroker provides the internal opeations of the streamer
func (p *HTTPSProxyStream) DoBroker(dest io.Writer, src io.Reader, end Notifier) {
	// _, ex := io.Copy(dest, src)
	//
	// if ex != nil {
	// 	go func() { p.errorend <- ex }()
	// }
	//
	// end <- struct{}{}
}

//DoBroker provides the internal opeations of the streamer
func (p *HTTPProxyStream) DoBroker(dest io.Writer, src io.Reader, end Notifier) {
	// _, ex := io.Copy(dest, src)
	//
	// if ex != nil {
	// 	go func() { p.errorend <- ex }()
	// }
	//
	// end <- struct{}{}
}
