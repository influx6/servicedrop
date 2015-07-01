package servicedrop

import (
	"io"
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
		dest      net.Conn
		src       net.Conn
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
		doBroker(io.Writer, io.Reader, Notifier)
	}

	//TCPProxyStream handles bare tcp stream proxying
	TCPProxyStream struct {
		*ProxyStream
	}
)

//reportEror helper to just log out err from a error channel
func reportError(n NotifierError) {
	for ex := range n {
		log.Printf("Recieved Error %+s", ex.Error())
	}
}

//NewProxyStream returns a new proxy streamer
func NewProxyStream(src, dest net.Conn) (p *ProxyStream) {
	p = &ProxyStream{
		src:       src,
		dest:      dest,
		closed:    make(Notifier),
		serverend: make(Notifier, 1),
		clientend: make(Notifier, 1),
		errorend:  make(NotifierError),
		do:        new(sync.Once),
		incoming:  flux.NewIdentityStream(),
		outgoing:  flux.NewIdentityStream(),
	}
	return
}

//empty broker implementation
func (p *ProxyStream) doBroker(d io.Writer, s io.Reader, r Notifier) {
}

//Close stops the streaming
func (p *ProxyStream) Close() error {
	p.do.Do(func() {
		close(p.closed)
	})
	return nil
}

//TCPStream provides a tcp proxy streamer
func TCPStream(src net.Conn, dest net.Conn) ProxyStreams {
	ts := &TCPStream{NewProxyStream(src, dest)}
	go ts.handle()
	return ts
}

//doBroker provides the internal opeations of the streamer
func (p *TCPProxyStream) doBroker(dest io.Writer, src io.Reader, end Notifier) {
	_, ex := io.Copy(dest, src)

	if ex != nil {
		go func() { errs <- ex }()
	}

	end <- struct{}{}
}

func (p *TCPProxyStream) handleProcess() {

	destwriter := io.MultiWriter(p.dest, p.outgoing)
	srcwriter := io.MultiWriter(p.src, p.incoming)

	go p.doBroker(destwriter, p.src, p.clientend)
	go p.doBroker(srcwriter, p.dest, p.serverend)

	go reportError(p.errorend)

	select {
	case <-p.closed:
		log.Println("Closing destination first streams!")
		p.serverend <- struct{}{}
	case <-p.clientend:
		//close the server and notifier serverend
		ex := p.dest.Close()
		if ex != nil {
			go func() { p.errorend <- ex }()
		}
		p.serverend <- struct{}{}
	case <-p.serverend:
		//close the client and notifier clientend
		ex := p.src.Close()
		if ex != nil {
			go func() { p.errorend <- ex }()
		}
		p.clientend <- struct{}{}
	}

}
