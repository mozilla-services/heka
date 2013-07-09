package testsupport

import (
	"fmt"
	"net"
	"net/http"
)

type OneHttpServer struct {
	response_data string
	listener      net.Listener
}

func NewOneHttpServer(response_data string, hostname string, port int) (server *OneHttpServer, e error) {
	l, e := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if e != nil {
		return nil, e
	}
	server = &OneHttpServer{response_data: response_data, listener: l}
	return server, nil
}

func (o *OneHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, o.response_data)
	o.listener.Close()
}

func (o *OneHttpServer) Start(urlpath string) {
	http.Handle(urlpath, o)
	http.Serve(o.listener, nil)
}
