package testsupport

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
)

type OneHttpServer struct {
	response_data string
	listener      net.Listener
}

// HTTP server that checks for Basic Authentication credentials
type HttpBasicAuthServer struct {
	username, password string
	listener           net.Listener
}

func NewOneHttpServer(response_data string, hostname string, port int) (server *OneHttpServer, e error) {
	l, e := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if e != nil {
		return nil, e
	}
	server = &OneHttpServer{response_data: response_data, listener: l}
	return server, nil
}

func NewHttpBasicAuthServer(username, password string, hostname string, port int) (server *HttpBasicAuthServer, e error) {
	l, e := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if e != nil {
		return nil, e
	}
	server = &HttpBasicAuthServer{username: username, password: password, listener: l}
	return server, nil
}

func (o *OneHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, o.response_data)
	o.listener.Close()
}

func (b *HttpBasicAuthServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(b.username+":"+b.password))
	if auth != r.Header.Get("Authorization") {
		http.Error(w, "Unauthorized", 401)
	}
	b.listener.Close()
}

func (o *OneHttpServer) Start(urlpath string) {
	http.Handle(urlpath, o)
	http.Serve(o.listener, nil)
}

func (b *HttpBasicAuthServer) Start(urlpath string) {
	http.Handle(urlpath, b)
	http.Serve(b.listener, nil)
}
