package pipeline

import (
	"container/list"
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
	mutex         sync.Mutex
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
	gl := &gracefulListener{Listener: l, manager: m, conns: list.New()}
	m.Set(lnet, laddr, &pluginListener{gracefulListener: gl, plugin: p})
	return gl, nil
}

func (m *netManager) Reset() {
	m.mutex.Lock()
	m.restarting = false
	m.mutex.Unlock()
}

func (m *netManager) Restart() {
	m.mutex.Lock()
	m.restarting = true
	m.listenerMutex.RLock()
	for _, l := range m.listeners {
		l.currentConn = l.conns.Front()
	}
	m.listenerMutex.RUnlock()
	m.mutex.Unlock()
}

type gracefulListener struct {
	net.Listener
	manager     *netManager
	conns       *list.List
	currentConn *list.Element
}

func (l *gracefulListener) Close() error {
	l.manager.mutex.Lock()
	defer l.manager.mutex.Unlock()
	if !l.manager.restarting {
		return l.Listener.Close()
	}
	return nil
}

func (l *gracefulListener) Accept() (net.Conn, error) {
	l.manager.mutex.Lock()
	defer l.manager.mutex.Unlock()
	if !l.manager.restarting {
		// This block causes Accept() to return the connections from the
		// previous run. These are cached after a restart so connections
		// aren't dropped. Once they've all been returned, we begin calling
		// the wrapped Listener's Accept() instead.
		if l.currentConn != nil {
			c := l.currentConn.Value.(net.Conn)
			l.currentConn = l.currentConn.Next()
			return c, nil
		}
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		c = &gracefulConn{Conn: c, listener: l}
		l.conns.PushBack(c)
		return c, nil
	}
	// During a restart we want to return an error so things calling Accept()
	// behave normally, and stop calling Accept()
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
	c.listener.manager.mutex.Lock()
	defer c.listener.manager.mutex.Unlock()
	if !c.listener.manager.restarting {
		// Remove ourself from the list
		for e := c.listener.conns.Front(); e != nil; e = e.Next() {
			if e.Value == c {
				c.listener.conns.Remove(e)
				break
			}
		}
		return c.Conn.Close()
	}
	return nil
}
