package main

import (
	"errors"
	"io"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

type sshCleanup struct {
	clients []*ssh.Client
}

func (c sshCleanup) Close() (err error) {
	defer func() { recover() }() // ignore any errors
	for _, client := range c.clients {
		defer func(client *ssh.Client) {
			e := client.Close()
			if err == nil {
				err = e
			}
		}(client)
	}
	return
}

func newClient(addrs []string, configs []*ssh.ClientConfig) (client *ssh.Client, cleanup io.Closer, err error) {
	// ensure same length!
	if len(addrs) != len(configs) {
		return nil, nil, errors.New("newClient: len(config) != len(addrs)")
	}

	// prepare a cleanup function
	c := sshCleanup{
		clients: make([]*ssh.Client, 0, len(addrs)),
	}

	if len(addrs) == 0 {
		return nil, nil, errors.New("no addresses provided")
	}

	// hop one by one
	for i, addr := range addrs {
		config := configs[i]
		log.Printf("Establishing new connection to %s", addr)

		// connect to the next hop in line
		client, err = connectHop(client, addr, config)
		if err != nil {
			log.Printf("Failed to connect to %s: %s", addr, err)
			c.Close()
			return nil, nil, err
		}

		// keep the old client around
		c.clients = append(c.clients, client)
	}

	log.Printf("Connected to final hop %s", addrs[len(addrs)-1])
	return client, c, nil
}

// connectHop connects to a single hopS
func connectHop(proxy *ssh.Client, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	// dial the network connection
	var conn net.Conn
	var err error
	if proxy == nil {
		conn, err = net.DialTimeout("tcp", addr, config.Timeout)
	} else {
		conn, err = proxy.Dial("tcp", addr)
	}
	// bail out if there was an error
	if err != nil {
		return nil, err
	}

	// make the client connection
	c, channels, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, err
	}

	// and finally the new ssh client
	return ssh.NewClient(c, channels, reqs), nil
}

// clientAlive checks if the connection to client is still active and sending data.
// when the client is non-nil, but not clientAlive, calls cleanup.Close()
func clientAlive(client *ssh.Client, cleanup io.Closer) bool {
	// no client => not alive
	if client == nil {
		return false
	}

	// check if actually alive
	if _, _, err := client.Conn.SendRequest("keepalive@golang.org", true, nil); err == nil {
		return true
	}

	// and return the cleanup
	log.Printf("Client connection is not responding, closing")
	if err := cleanup.Close(); err != nil {
		log.Printf("Failed to close client connection: %s", err)
	}
	return false
}
