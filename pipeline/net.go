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

type pluginListener struct {
	net.Listener
	Plugin
}

type ListenerManager struct {
	m map[string]*pluginListener
	sync.RWMutex
}

func NewListenerManager() *ListenerManager {
	return &ListenerManager{m: make(map[string]*pluginListener)}
}

func (m *ListenerManager) Get(lnet, laddr string) (l *pluginListener) {
	m.RLock()
	l = m.m[lnet+laddr]
	m.RUnlock()
	return
}

func (m *ListenerManager) Set(lnet, laddr string, l *pluginListener) {
	m.Lock()
	m.m[lnet+laddr] = l
	m.Unlock()
	return
}

func (m *ListenerManager) Listen(lnet, laddr string, p Plugin) (net.Listener, error) {
	pl := m.Get(lnet, laddr)
	if pl != nil {
		pl.Plugin = p
		return pl.Listener, nil
	}
	return NewListener(lnet, laddr, p, m)
}

func NewListener(lnet, laddr string, p Plugin, m *ListenerManager) (net.Listener, error) {
	l, err := net.Listen(lnet, laddr)
	if err != nil {
		return nil, err
	}
	m.Set(lnet, laddr, &pluginListener{Listener: l, Plugin: p})
	return l, nil
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

func (m *netManager) Listen(lnet, laddr string, p Plugin) (net.Listener, error) {
	l, err := m.manager.Listen(lnet, laddr, p)
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
