package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"testing"
)

func TestEcho(t *testing.T) {
	port, close := newEchoServer()
	defer close()

	c, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf(err.Error())
	}
	input := "foo"

	err = writeLine(c, input)
	if err != nil {
		t.Fatal("cound not write to conn", err.Error())
	}

	out, err := readLine(c)
	if err != nil {
		t.Errorf(err.Error())
	}

	if got, want := out, input; got != want {
		t.Errorf("got %q, want %q from echo server via conn pool", got, want)
	}
}

func TestPool(t *testing.T) {
	port, close := newEchoServer()
	defer close()
	cp := &ConnPool{}
	p, err := cp.New(fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, err := p.Get()
	if err != nil {
		t.Fatalf(err.Error())
	}

	input := "foo"

	err = writeLine(c, input)
	if err != nil {
		t.Fatal("cound not write to conn", err.Error())
	}

	out, err := readLine(c)
	if err != nil {
		t.Errorf(err.Error())
	}

	if got, want := out, input; got != want {
		t.Errorf("got %q, want %q from echo server via conn pool", got, want)
	}
}

func TestPoolCanGrowAndAllConnsWork(t *testing.T) {
	port, close := newEchoServer()
	defer close()
	cp := &ConnPool{}
	p, err := cp.New(fmt.Sprintf(":%d", port))
	if err != nil {
		t.Errorf(err.Error())
	}

	c1, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}

	c2, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}

	c3, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}

	c4, err := p.Get()
	if err != nil {
		t.Errorf("should have been able to get more connections than max from pool")
	}

	addrs := make(map[string]struct{})
	addrs[fmt.Sprintf("%p", c1)] = struct{}{}
	addrs[fmt.Sprintf("%p", c2)] = struct{}{}
	addrs[fmt.Sprintf("%p", c3)] = struct{}{}
	addrs[fmt.Sprintf("%p", c4)] = struct{}{}

	tests := []struct {
		connName string
		c        net.Conn
		input    string
		expected string
	}{
		{"c1", c1, "a", "a"},
		{"c2", c2, "b", "b"},
		{"c3", c3, "c", "c"},
		{"c4", c4, "d", "d"},
	}

	for _, test := range tests {
		if err := writeLine(test.c, test.input); err != nil {
			t.Errorf("should be able to write to %s, got error %v", test.connName, err)
		}
	}

	for _, test := range tests {
		got, err := readLine(test.c)
		if err != nil {
			t.Errorf("test %s - readLine error %v", test.connName, err)
		}
		if got != test.expected {
			t.Errorf("test %s - got %q, want %q", test.connName, got, test.expected)
		}
	}

	if p.Len() != 0 {
		t.Errorf("got %d, but expected no idle conns, we checked out more than max pool size", p.Len())
	}
	c4.Close()
	if p.Len() != 1 {
		t.Errorf("got %d, because we returned one", p.Len())
	}
}

func TestPoolConnReuse(t *testing.T) {
	port, close := newEchoServer()
	defer close()
	cp := &ConnPool{}
	p, err := cp.New(fmt.Sprintf(":%d", port))
	if err != nil {
		t.Errorf(err.Error())
	}

	c1, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}
	writeLine(c1, "foo")

	c2, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}
	writeLine(c2, "foo")

	c3, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}
	writeLine(c2, "foo")

	// recycle some more conns
	c3.Close()
	c2.Close()

	c4, err := p.Get()
	if err != nil {
		t.Errorf("should have been able to get a new conn - %v", err.Error())
	}

	c5, err := p.Get()
	if err != nil {
		t.Errorf("should have been able to get a new conn - %v", err.Error())
	}

	_, _, _, _, _ = c1, c2, c3, c4, c5

	cp.mu.Lock()
	defer cp.mu.Unlock()
	if got, want := cp.dialCount, 3; got != want {
		t.Errorf("got %d dials, want %d", got, want)
	}
}

func TestConnectionBackgroundClose(t *testing.T) {
	t.Skip("the conn.Close() method here is overridden to return the conn to the pool. I'm not sure yet how to show that conn actually closing. Maybe have a kill command in the echo server where if it receives a given string, it kills the conn. Probably the only way.")
	port, close := newEchoServer()
	defer close()
	cp := &ConnPool{}
	p, err := cp.New(fmt.Sprintf(":%d", port))
	if err != nil {
		t.Errorf(err.Error())
	}

	c1, err := p.Get()
	if err != nil {
		t.Errorf(err.Error())
	}

	err = c1.Close()
	if err != nil {
		t.Error("unable to close connection", err.Error())
	}
	err = writeLine(c1, "foo")
	if err != nil {
		t.Error("unable to write to connection", err.Error())
	}

	for i := 0; i < 100; i++ {
		c, _ := p.Get()
		writeLine(c, fmt.Sprintf("i-%d", i))
		c.Close()
	}

	for i := 0; i <= 100; i++ {
		c, _ := p.Get()
		writeLine(c, fmt.Sprintf("i-%d", i))
		c.Close()
	}

	for err != nil {
		var s string
		s, err = readLine(c1)
		fmt.Println("read:", s)
	}

	t.Fail()
}

func newEchoServer() (port int, closeFn func()) {
	l, err := net.Listen("tcp", ":0")
	if l == nil {
		panic("couldn't start listening: " + err.Error())
	}

	closeFn = func() { /* do anything here? */ }

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Println("error accepting new connection ", err.Error())
			}

			go handleRequest(conn)
		}
	}()

	return l.Addr().(*net.TCPAddr).Port, closeFn
}

func handleRequest(conn net.Conn) {
	defer conn.Close()

	for {
		buf := make([]byte, 1024)
		size, err := conn.Read(buf)
		if err != nil {
			return
		}
		data := buf[:size]
		conn.Write(data)
	}
}

func readLine(c net.Conn) (string, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 256)
	for {
		n, err := c.Read(tmp)
		if err != nil {
			if err != io.EOF {
				return "", err
			}
			break
		}
		buf = append(buf, tmp[:n]...)
		if len(buf) > 1 && buf[len(buf)-1] == '\n' {
			buf = buf[0 : len(buf)-1]
			break
		}
	}

	return string(buf), nil
}

func writeLine(c net.Conn, s string) error {
	_, err := c.Write([]byte(s + "\n"))
	return err
}
