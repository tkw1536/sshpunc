package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

func main() {
	// parse hostname
	user, host, err := splitHost(sshHost)
	if err != nil {
		log.Fatalf("Unable to parse hostname %s: %s", sshHost, err)
	}

	// read the private key
	pk, err := readPrivateKey(sshKey)
	if err != nil {
		log.Fatalf("Unable to read private key %s: %s", sshKey, err)
	}
	log.Printf("Loaded private key from %s", sshKey)

	// setup a connection (we're not going to use it for a bit)
	// this might already be gone by the time someone connects, but that does not matter.
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			pk,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	go connect(host, config)

	// start listening locally
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		log.Fatalf("Unable to listen on %s: %s", localAddr, err)
	}
	defer listener.Close()

	// accept all the connections
	go func() {
		log.Printf("Forwarding %s -> %s", localAddr, remoteAddr)
		for {

			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Unable to accept listener: %s", err)
				continue
			}

			go forward(conn, remoteAddr, host, config)
		}
	}()

	// wait for the end
	<-globalContext.Done()
	log.Print("shutting down")
}

//
// PARSER
//

func splitHost(host string) (user, addr string, err error) {
	if !strings.ContainsRune(host, '@') {
		return "", "", errors.New("host does not contain a username")
	}

	parts := strings.SplitN(host, "@", 2)
	user = parts[0]
	addr = parts[1]

	// cheap check to make sure address contains a port
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	return
}

func readPrivateKey(path string) (ssh.AuthMethod, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(bytes)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

//
// ssh connection
//

var client *ssh.Client
var clientMutex sync.Mutex

func connect(addr string, config *ssh.ClientConfig) (c *ssh.Client, e error) {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// if we already have a client send a keepalive packet
	// and re-use the client if we can!
	if client != nil {
		_, _, err := client.Conn.SendRequest("keepalive@golang.org", true, nil)
		if err == nil {
			return client, nil
		}
	}

	// we need to make a new connection!
	log.Printf("Establishing new connection to %s", addr)
	var err error
	client, err = ssh.Dial("tcp", addr, config)
	return client, err
}

//
// forwarding
//

var waitPool = &sync.Pool{
	New: func() interface{} {
		return new(sync.WaitGroup)
	},
}

func forward(conn net.Conn, remoteAddr string, addr string, config *ssh.ClientConfig) {
	defer conn.Close()

	// get or make a new client
	client, err := connect(addr, config)
	if err != nil {
		log.Printf("Failed to connect to %s: %s", addr, err)
		return
	}

	// make a new connection to the remote address
	dest, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("Failed to dial %s: %s", remoteAddr, err)
		return
	}

	// copy input / output

	wg := waitPool.Get().(*sync.WaitGroup)
	wg.Add(2)

	go func() {
		defer wg.Done()

		_, err = io.Copy(dest, conn)
		if err != nil {
			log.Printf("Failed to copy output stream: %s", err)
		}
	}()

	go func() {
		defer wg.Done()

		_, err = io.Copy(conn, dest)
		if err != nil {
			log.Printf("Failed to copy input stream: %s", err)
		}
	}()

	// wait for the connection to be done
	// to close the connection properly!
	wg.Wait()
	waitPool.Put(wg)
}

//
// ctrl+c
//

var globalContext context.Context

func init() {
	var cancel context.CancelFunc
	globalContext, cancel = context.WithCancel(context.Background())

	cancelChan := make(chan os.Signal)
	signal.Notify(cancelChan, os.Interrupt)

	go func() {
		<-cancelChan
		cancel()
	}()
}

//
// FLAGS
//

var sshHost string
var sshKey string
var localAddr string
var remoteAddr string

func init() {
	defer flag.Parse()

	flag.StringVar(&sshHost, "sshhost", os.Getenv("SSHHOST"), "ssh host to connect to ($SSHHOST)")
	flag.StringVar(&sshKey, "sshkey", os.Getenv("SSHKEY"), "ssh key file to read")
	flag.StringVar(&localAddr, "localaddr", os.Getenv("LOCALADDR"), "local address to listen on")
	flag.StringVar(&remoteAddr, "remoteaddr", os.Getenv("REMOTEADDR"), "remote address to forward to")
}
