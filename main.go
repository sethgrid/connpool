package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/fatih/pool"
)

func main() {
	fmt.Println("stub")
}

type ConnPool struct {
	mu        sync.Mutex
	dialCount int
}

func (p *ConnPool) New(addr string) (pool.Pool, error) {
	initConnCount := 1
	maxConnCount := 3

	return pool.NewChannelPool(initConnCount, maxConnCount, p.newPoolFactory(addr))
}

func (p *ConnPool) newPoolFactory(addr string) pool.Factory {
	var conFactory pool.Factory = func() (net.Conn, error) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.dialCount++
		return net.Dial("tcp", addr)
	}
	return conFactory
}
