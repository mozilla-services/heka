package pipeline

import (
	"errors"
	"net"
	"sync"
	"time"
)

var globalNetManager *netManager

type Deadliner interface {
	SetDeadline(t time.Time) error
}

type ListenerManager struct {
	m map[string]net.Listener
	sync.RWMutex
}

func NewListenerManager() *ListenerManager {
	return &ListenerManager{m: make(map[string]net.Listener)}
}

func (m *ListenerManager) Get(lnet, laddr string) (l net.Listener) {
	m.RLock()
	l = m.m[lnet+laddr]
	m.RUnlock()
	return
}

func (m *ListenerManager) Set(lnet, laddr string, l net.Listener) {
	m.Lock()
	m.m[lnet+laddr] = l
	m.Unlock()
	return
}

func (m *ListenerManager) Listeners() []net.Listener {
	listeners := make([]net.Listener, 0, len(m.m))
	for _, l := range m.m {
		listeners = append(listeners, l)
	}
	return listeners
}

func (m *ListenerManager) Listen(lnet, laddr string) (net.Listener, error) {
	l := m.Get(lnet, laddr)
	if l != nil {
		return l, nil
	}
	return NewListener(lnet, laddr, m)
}

func NewListener(lnet, laddr string, m *ListenerManager) (l net.Listener, err error) {
	l, err = net.Listen(lnet, laddr)
	if err != nil {
		return
	}
	m.Set(lnet, laddr, l)
	return
}

func NetManager() *netManager {
	return globalNetManager
}

func newGlobalNetManager() (*netManager, error) {
	if globalNetManager != nil {
		return nil, errors.New("Global NetManager already created.")
	}
	globalNetManager = &netManager{
		manager: NewListenerManager(),
	}
	return globalNetManager, nil
}

type netManager struct {
	manager    *ListenerManager
	restarting bool
}

func (m *netManager) Listen(lnet, laddr string) (net.Listener, error) {
	l, err := m.manager.Listen(lnet, laddr)
	if err != nil {
		return nil, err
	}
	l = &gracefulListener{Listener: l, manager: m}
	return l, nil
}

type gracefulListener struct {
	net.Listener
	manager *netManager
}

func (l *gracefulListener) Close() error {
	if !l.manager.restarting {
		return l.Listener.Close()
	}
	return nil
}

func (l *gracefulListener) Accept() (net.Conn, error) {
	if !l.manager.restarting {
		return l.Listener.Accept()
	}
	return nil, errors.New("restarting")
}

func (l *gracefulListener) SetDeadline(t time.Time) error {
	if dl, ok := l.Listener.(Deadliner); ok {
		return dl.SetDeadline(t)
	}
	return nil
}
