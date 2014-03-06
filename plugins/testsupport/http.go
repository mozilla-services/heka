package testsupport

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
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

// HTTP server that checks whether the correct HTTP method was used in the request
type HttpMethodServer struct {
	method   string
	listener net.Listener
}

// HTTP server that checks whether the correct headers were set in the request
type HttpHeadersServer struct {
	headers  map[string]string
	listener net.Listener
}

// HTTP server that echoes back the request body if there is one
type HttpBodyServer struct {
	listener net.Listener
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

func NewHttpMethodServer(method string, hostname string, port int) (server *HttpMethodServer, e error) {
	l, e := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if e != nil {
		return nil, e
	}
	server = &HttpMethodServer{method: method, listener: l}
	return server, nil
}

func NewHttpHeadersServer(headers map[string]string, hostname string, port int) (server *HttpHeadersServer, e error) {
	l, e := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if e != nil {
		return nil, e
	}
	server = &HttpHeadersServer{headers: headers, listener: l}
	return server, nil
}

func NewHttpBodyServer(hostname string, port int) (server *HttpBodyServer, e error) {
	l, e := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if e != nil {
		return nil, e
	}
	server = &HttpBodyServer{listener: l}
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

func (m *HttpMethodServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.method != r.Method {
		http.Error(w, "Method not allowed", 405)
	}
	m.listener.Close()
}

func (h *HttpHeadersServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for k, v := range h.headers {
		if r.Header.Get(k) != v {
			http.Error(w, "Bad Request", 400)
			break
		}
	}
	h.listener.Close()
}

func (b *HttpBodyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err == nil {
		fmt.Fprintf(w, string(body))
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

func (m *HttpMethodServer) Start(urlpath string) {
	http.Handle(urlpath, m)
	http.Serve(m.listener, nil)
}

func (h *HttpHeadersServer) Start(urlpath string) {
	http.Handle(urlpath, h)
	http.Serve(h.listener, nil)
}

func (b *HttpBodyServer) Start(urlpath string) {
	http.Handle(urlpath, b)
	http.Serve(b.listener, nil)
}
