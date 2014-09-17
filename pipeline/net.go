package pipeline

import (
	"errors"
	"net"
	"sync"
	"time"
)

var globalNetManager *netManager

type netManager struct {
	listeners     map[string]*pluginListener
	listenerMutex sync.RWMutex
	restarting    bool
	restartMutex  sync.RWMutex
}

type Deadliner interface {
	SetDeadline(t time.Time) error
}

type pluginListener struct {
	*gracefulListener
	plugin Plugin
}

func NetManager() *netManager {
	return globalNetManager
}

func newGlobalNetManager() (*netManager, error) {
	if globalNetManager != nil {
		return nil, errors.New("Global NetManager already created.")
	}
	globalNetManager = NewNetManager()
	return globalNetManager, nil
}

func NewNetManager() *netManager {
	return &netManager{listeners: make(map[string]*pluginListener)}
}

func (m *netManager) Get(lnet, laddr string) (l *pluginListener) {
	m.listenerMutex.RLock()
	l = m.listeners[lnet+laddr]
	m.listenerMutex.RUnlock()
	return
}

func (m *netManager) Set(lnet, laddr string, l *pluginListener) {
	m.listenerMutex.Lock()
	m.listeners[lnet+laddr] = l
	m.listenerMutex.Unlock()
	return
}

func (m *netManager) Listen(lnet, laddr string, p Plugin) (net.Listener, error) {
	pl := m.Get(lnet, laddr)
	if pl != nil {
		pl.plugin = p
		return pl.gracefulListener, nil
	}
	return NewListener(lnet, laddr, p, m)
}

func NewListener(lnet, laddr string, p Plugin, m *netManager) (net.Listener, error) {
	l, err := net.Listen(lnet, laddr)
	if err != nil {
		return nil, err
	}
	gl := &gracefulListener{Listener: l, manager: m}
	m.Set(lnet, laddr, &pluginListener{gracefulListener: gl, plugin: p})
	return gl, nil
}

func (m *netManager) Reset() {
	m.restartMutex.Lock()
	m.restarting = false
	m.restartMutex.Unlock()
}

func (m *netManager) Restart() {
	m.restartMutex.Lock()
	m.restarting = true
	m.restartMutex.Unlock()
}

type gracefulListener struct {
	net.Listener
	manager *netManager
}

func (l *gracefulListener) Close() error {
	l.manager.restartMutex.Lock()
	defer l.manager.restartMutex.Unlock()
	if !l.manager.restarting {
		return l.Listener.Close()
	}
	return nil
}

func (l *gracefulListener) Accept() (net.Conn, error) {
	l.manager.restartMutex.RLock()
	defer l.manager.restartMutex.RUnlock()
	if !l.manager.restarting {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		c = &gracefulConn{Conn: c, listener: l}
		return c, nil
	}
	return nil, errors.New("restarting")
}

func (l *gracefulListener) SetDeadline(t time.Time) error {
	if dl, ok := l.Listener.(Deadliner); ok {
		return dl.SetDeadline(t)
	}
	return nil
}

type gracefulConn struct {
	net.Conn
	listener *gracefulListener
}

func (c *gracefulConn) Close() error {
	c.listener.manager.restartMutex.RLock()
	defer c.listener.manager.restartMutex.RUnlock()
	if !c.listener.manager.restarting {
		return c.Conn.Close()
	}
	return nil
}
