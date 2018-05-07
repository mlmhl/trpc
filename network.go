package trpc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Network is an alternative for the real network environment, which can be used to
// simulate request/response lost, messages delay and network partition.
// NOTE: There's no actual network connections established if you use this Network.
type Network struct {
	lock sync.Mutex

	servers     map[string]*Server
	clients     map[string]*Client
	addressMap  map[string]string // Map network address to server name.
	connections map[string]string // Map client name to server name it connected to.
}

func (n *Network) NewServer() *Server {
	n.lock.Lock()
	defer n.lock.Unlock()

	name := generateName("server", len(n.servers))
	server := newServer(name, n.registerServer)
	n.servers[name] = server

	return server
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
	delete(n.clients, name)
	return nil
}

func (n *Network) dispatch(
	serverName, clientName string,
	service, method string,
	args, reply interface{}) error {
	server, err := n.getServer(serverName)
	if err != nil {
		return err
	}
	return server.dispatch(service, method, args, reply)
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
