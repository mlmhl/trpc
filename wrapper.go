// +build product

/*
This file is a simple wrapper of net/rpc package, which is the default behavior.
It means that if you use this package and didn't add the rpc-test tag when you
build your application, this package is the same with net/rpc package.
*/

package trpc

import "net/rpc"

type Client = rpc.Client

type Server = rpc.Server

var NewServer = rpc.NewServer

var Dial = rpc.Dial
var NewClient = rpc.NewClient