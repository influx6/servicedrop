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
		serverend Notifer
		errorend  Notifier
		do        *sync.Once
		incoming  flux.StackStreamers
		outgoing  flux.StackStreamers
	}
)

//Close stops the streaming
func (p *ProxyStream) Close() {
	p.do.Do(func() {
		close(p.closer)
	})
}

//Close stops the streaming
func (p *ProxyStream) handleProcess() {

	destwriter := io.MultiWriter(p.dest, p.outgoing)
	srcwriter := io.MultiWriter(p.src, p.incoming)

	// destwriter := p.dest
	// srcwriter := p.src

	go doBrocker(destwriter, p.src, p.clientend, p.errorend)
	go doBrocker(srcwriter, p.dest, p.serverend, p.errorend)
	go reportError(p.errorend)

	select {
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

//ProxyConn provides a simple function call insteadof using NewProxyStream,
//just convienience method
func ProxyConn(src net.Conn, dest net.Conn) *ProxyStream {
	return NewProxyStream(src, dest)
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

	go p.handleProcess()

	return
}

func reportError(n NotifierError) {
	for ex := range n {
		log.Printf("Recieved Error %+s", ex.Error())
	}
}

func doBrocker(dest, src net.Conn, ender, errs Notifer) {
	_, ex := io.Copy(dest, src)

	if err != nil {
		go func() { errs <- ex }()
	}

	// erx := src.Close()
	// if erx != nil {
	// 	go func() { errs <- erx }()
	// }

	enders <- struct{}{}
}
