package servicedrop

import (
	"net/http"
)

type (

	//HTTPProtocol defines the struct for http based services
	HTTPProtocol struct {
		*Protocol
		cert *HTTPCert
	}

	//HTTPCert defines the certificate information for a https connection
	HTTPCert struct {
		Key  string
		Cert string
	}
)

//Dial connects the protocol and makes it ready for use
func (m *HTTPProtocol) Dial() error {
	if m.cert != nil {
		return http.ListenAndServe(m.Descriptor().Host(),
			http.HandlerFunc(m.ProcessRequests))
	}
	return http.ListenAndServeTLS(m.Descriptor().Host(), m.cert.Cert, m.cert.Key,
		http.HandlerFunc(m.ProcessRequests))
}

//ProcessRequests handles the management of operations by the http packet
func (m *HTTPProtocol) ProcessRequests(res http.ResponseWriter, req *http.Request) {
	m.Routes().Serve(req.URL.String(), CollectHTTPBody(req, res), m.conf.timeout)
}
