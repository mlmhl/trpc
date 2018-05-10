package trpc

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	longTimeout  = 7000
	shortTimeout = 100
	shortDelay   = 27
)

var timeoutErr = errors.New("rpc: request timeout")

func NewNetwork() *Network {
	return &Network{
		servers:     make(map[string]*Server),
		clients:     make(map[string]*Client),
		addressMap:  make(map[string]string),
		connections: make(map[string]string),
		enabled:     make(map[string]bool),
		reliable:    true,
	}
}

// Network is an alternative for the real network environment, which can be used to
// simulate request/response lost, messages delay and network partition.
// NOTE: There's no actual network connections established if you use this Network.
type Network struct {
	lock sync.Mutex

	servers     map[string]*Server
	clients     map[string]*Client
	addressMap  map[string]string // Map network address to server name.
	connections map[string]string // Map client name to server name it connected to.

	// If Network is not reliable, requests maybe delayed or even dropped.
	reliable bool
	// If longDelay is true, requests may suffer a long delay before timeout.
	longDelay bool
	// If longReorder is true, requests may suffer a long delay before response.
	longReorder bool

	// If a client isn't enabled, requests won't be replied an eventually timeout.
	enabled map[string]bool
}

// SetReliable marks Network as reliable or not reliable.
func (n *Network) SetReliable(reliable bool) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.reliable = reliable
}

// EnableClient enables a client with specified name.
func (n *Network) EnableClient(name string) {
	n.setClientEnable(name, true)
}

// DisableClient disables a client with specified name.
func (n *Network) DisableClient(name string) {
	n.setClientEnable(name, false)
}

func (n *Network) setClientEnable(name string, enabled bool) {
	n.lock.Lock()
	defer n.lock.Unlock()
	if _, exist := n.clients[name]; !exist {
		// Do nothing if client not exist.
		return
	}
	n.enabled[name] = enabled
}

// NewServer creates a Server with generated name.
func (n *Network) NewServer() *Server {
	n.lock.Lock()
	defer n.lock.Unlock()

	name := generateName("server", len(n.servers))
	server := newServer(name, n.registerServer)
	n.servers[name] = server

	return server
}

func (n *Network) RemoveServer(server *Server) {
	n.lock.Lock()
	defer n.lock.Unlock()
	delete(n.servers, server.name)
}

// Bind the network address to a server. If the address is already occupied, no nothing.
// TODO: Inform the user if address is occupied.
func (n *Network) registerServer(name, network, address string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	key := joinAddress(network, address)
	if _, exist := n.addressMap[key]; exist {
		return
	}
	n.addressMap[key] = name
}

// Dail creates a new Client connects to the specified network address.
func (n *Network) Dail(network, address string) (*Client, error) {
	server, err := n.getServerByAddress(network, address)
	if err != nil {
		return nil, err
	}
	return n.createClient(server), nil
}

func (n *Network) getServerByAddress(network, address string) (*Server, error) {
	name, exist := n.addressMap[joinAddress(network, address)]
	if exist {
		server, exist := n.servers[name]
		if exist {
			return server, nil
		}
	}
	return nil, fmt.Errorf("rpc: no server listens on %s/%s", network, address)
}

func (n *Network) createClient(server *Server) *Client {
	n.lock.Lock()
	defer n.lock.Unlock()

	name := generateName("client", len(n.clients))
	client := newClient(name, n.call, n.closeClient)

	n.clients[name] = client
	// Enable the client by default.
	n.enabled[name] = true
	n.connections[name] = server.name

	return client
}

func (n *Network) call(clientName, serviceMethod string, args interface{}, reply interface{}) error {
	service, method, err := parseServiceMethod(serviceMethod)
	if err != nil {
		return err
	}
	serverName, exist := n.connections[clientName]
	if !exist {
		// This is odd.
		return fmt.Errorf("rpc: client doesn't connect to any server")
	}
	return n.dispatch(serverName, clientName, service, method, args, reply)
}

func (n *Network) closeClient(name string) error {
	n.lock.Lock()
	defer n.lock.Unlock()
	_, exist := n.clients[name]
	if exist {
		return errors.New("rpc: client not exist")
	}
	delete(n.enabled, name)
	delete(n.clients, name)
	return nil
}

func (n *Network) dispatch(
	serverName, clientName string,
	service, method string,
	args, reply interface{}) error {
	server, err := n.getServer(serverName)
	enabled, reliable, longDelay, longRecorder := n.networkCondition(clientName)

	if !enabled || err != nil {
		// Client is disabled or server is removed, treated as no reply and eventual timeout.
		timeout := 0
		if longDelay {
			timeout = rand.Int() % longTimeout
		} else {
			timeout = rand.Int() % shortTimeout
		}
		time.Sleep(time.Duration(timeout) * time.Millisecond)
		return timeoutErr
	}

	if !reliable {
		if msgLost() {
			// Drop the request and return as timeout.
			return timeoutErr
		}
		// Simulate a short delay
		time.Sleep(time.Duration(rand.Int()%shortDelay) * time.Millisecond)
	}

	err = server.dispatch(service, method, args, reply)

	if !reliable && msgLost() {
		// Drop thr response and return as timeout.
		return timeoutErr
	}
	if longRecorder {
		time.Sleep(time.Duration(responseDelay()) * time.Millisecond)
	}

	return err
}

func (n *Network) networkCondition(clientName string) (bool, bool, bool, bool) {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.enabled[clientName], n.reliable, n.longDelay, n.longReorder
}

func (n *Network) getServer(name string) (*Server, error) {
	server, exist := n.servers[name]
	if !exist {
		return nil, errors.New("rpc: server missed")
	}
	return server, nil
}

func generateName(prefix string, index int) string {
	return fmt.Sprintf("%s-%d", prefix, index)
}

func joinAddress(network, address string) string {
	return fmt.Sprintf("%s-%s", network, address)
}

func parseServiceMethod(serviceMethod string) (string, string, error) {
	tags := strings.Split(serviceMethod, ".")
	if len(tags) != 2 {
		return "", "", fmt.Errorf("rpc: invalid service methods: %s", serviceMethod)
	}
	return tags[0], tags[1], nil
}

// The probability of request lost is 1/10.
func msgLost() bool {
	return rand.Int()%1000 < 100
}

func responseDelay() int {
	return 200 + rand.Intn(1+rand.Intn(2000))
}
