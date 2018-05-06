package trpc

import (
	"fmt"
	"net"
	"reflect"
	"sync"
)

type registrar func(name, network, address string)

type service struct {
	lock     sync.Mutex
	name     string                    // name of service
	typ      reflect.Type              // type of the receiver
	receiver reflect.Value             // receiver of methods for the service
	methods  map[string]reflect.Method // registered methods
}

func (s *service) dispatch(method string, args, reply interface{}) error {
	m, err := s.getMethod(method)
	if err != nil {
		return err
	}
	m.Func.Call([]reflect.Value{s.receiver, reflect.ValueOf(args), reflect.ValueOf(reply)})
	return nil
}

func (s *service) addMethods(methods []reflect.Method) {
	s.lock.Lock()
	defer s.lock.Unlock()
	for i := range methods {
		s.methods[methods[i].Name] = methods[i]
	}
}

func (s *service) getMethod(name string) (reflect.Method, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	m, exist := s.methods[name]
	if !exist {
		return reflect.Method{}, fmt.Errorf("rpc: Can't find method %s of service %s", name, s.name)
	}
	return m, nil
}

// Server is an alternative of native Server in net/rpc for test. You can simulate
// request/response lost, messages delay and network partition with this server.
// NOTE: There's no actual network connections established if you use this server.
type Server struct {
	lock sync.Mutex
	// Unique name to identify this client.
	name string
	// Server uses register to register itself in Network.
	registrar registrar
	// Services register in this server.
	services map[string]*service
}

func newServer(name string, registrar registrar) *Server {
	return &Server{
		name:      name,
		registrar: registrar,
		services:  make(map[string]*service),
	}
}

func (server *Server) Accept(listener net.Listener) {
	server.registrar(server.name, listener.Addr().Network(), listener.Addr().String())
}

func (server *Server) Register(rcvr interface{}) error {
	return server.RegisterName("", rcvr)
}

func (server *Server) RegisterName(name string, rcvr interface{}) error {
	s := &service{
		typ:      reflect.TypeOf(rcvr),
		receiver: reflect.ValueOf(rcvr),
		methods:  make(map[string]reflect.Method),
	}
	if len(name) == 0 {
		name = reflect.Indirect(s.receiver).Type().Name()
	}

	methods := make([]reflect.Method, s.typ.NumMethod())

	// Register all methods.
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		// The methods should be exported and has valid signature.
		if method.PkgPath == "" && validMethodType(method.Type) {
			methods[i] = method
		}
	}
	if len(methods) == 0 {
		return fmt.Errorf("rpc.Register: type %s has no exported methods", s.typ.Name())
	}
	s.addMethods(methods)

	return server.addService(name, s)
}

func (server *Server) addService(name string, s *service) error {
	server.lock.Lock()
	defer server.lock.Unlock()
	if _, exist := server.services[name]; exist {
		return fmt.Errorf("rpc: service already defined: %s", name)
	}
	server.services[name] = s
	return nil
}

func (server *Server) getService(name string) (*service, error) {
	server.lock.Lock()
	defer server.lock.Unlock()
	s, exist := server.services[name]
	if !exist {
		return nil, fmt.Errorf("rpc: service %s undefined", name)
	}
	return s, nil
}

func (server *Server) dispatch(service, method string, args, reply interface{}) error {
	srv, err := server.getService(service)
	if err != nil {
		return nil
	}
	return srv.dispatch(method, args, reply)
}

func validMethodType(typ reflect.Type) bool {
	if typ.NumIn() != 3 || typ.NumOut() != 0 {
		return false
	}

	// All input args should be pointer.
	for i := 1; i < typ.NumIn(); i++ {
		if typ.In(i).Kind() != reflect.Ptr {
			return false
		}
	}

	return true
}
