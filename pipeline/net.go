package pipeline

import (
	"container/list"
	"errors"
	"net"
	"sync"
	"time"
)

// TODO: move most logic from inside the methods to a separate go routine.
// Currently there is a lot of shared, mutable state, and many mutexes.

var globalNetManager *netManager

type netManager struct {
	// listeners key is the type of connection + the addr, and the value is
	// the listener we're maintaining, as well as the plugin associated.
	// if the plugin is nil after we've began listening, then it means the
	// listener is no longer valid and needs to be cleaned up
	listeners     map[string]*pluginListener
	listenerMutex sync.RWMutex
	restarting    bool
	mutex         sync.RWMutex
}

type Deadliner interface {
	SetDeadline(t time.Time) error
}

type pluginListener struct {
	*gracefulListener
	plugin Plugin
}

func NetManager() (*netManager, error) {
	if globalNetManager == nil {
		return newGlobalNetManager()
	}
	return globalNetManager, nil
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
	gl := &gracefulListener{Listener: l, manager: m, conns: list.New(), net: lnet, addr: laddr}
	m.Set(lnet, laddr, &pluginListener{gracefulListener: gl, plugin: p})
	return gl, nil
}

func (m *netManager) Reset() {
	m.mutex.Lock()
	m.restarting = false
	m.mutex.Unlock()

	m.listenerMutex.Lock()
	for _, listener := range m.listeners {
		listener.plugin = nil
	}
	m.listenerMutex.Unlock()
}

func (m *netManager) Restart() {
	m.mutex.Lock()
	m.restarting = true
	m.mutex.Unlock()
	// When we start the restart process, we need to set the currentConn
	// for each listener, so they begin returning connections they had
	// before we restarted
	m.listenerMutex.RLock()
	for _, l := range m.listeners {
		l.currentConn = l.conns.Front()
	}
	m.listenerMutex.RUnlock()
}

func (m *netManager) Restarting() (restarting bool) {
	m.mutex.RLock()
	restarting = m.restarting
	m.mutex.RUnlock()
	return
}

func (m *netManager) CloseUnusedListeners() error {
	errs := make([]error, 0)
	for _, listener := range m.listeners {
		if listener.plugin == nil {
			// Close the listener
			err := listener.Close()
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return MultiError(errs)
}

type gracefulListener struct {
	net.Listener
	manager     *netManager
	conns       *list.List
	currentConn *list.Element
	net, addr   string
}

func (l *gracefulListener) Close() error {
	// the listener only actually closed if we're not restarting
	if !l.manager.Restarting() {
		// we should remove ourself from the map of listeners.
		l.manager.listenerMutex.Lock()
		delete(l.manager.listeners, l.Key())
		l.manager.listenerMutex.Unlock()

		// Close our connections
		err := l.CloseConns()
		if err != nil {
			return err
		}

		// Actually close the listener
		return l.Listener.Close()
	}
	return nil
}

func (l *gracefulListener) CloseConns() error {
	errs := make([]error, 0)
	// Close all connections for this listener
	for e := l.conns.Front(); e != nil; e = e.Next() {
		conn := e.Value.(net.Conn)
		err := conn.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return MultiError(errs)
}

func (l *gracefulListener) Accept() (net.Conn, error) {
	if !l.manager.Restarting() {
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

func (l *gracefulListener) Key() string {
	return l.net + l.addr
}

type gracefulConn struct {
	net.Conn
	listener *gracefulListener
}

func (c *gracefulConn) Close() error {
	// We only close connections if we're not restarting
	if !c.listener.manager.Restarting() {
		// Remove ourself from the list of connections
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
