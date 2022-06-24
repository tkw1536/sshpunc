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
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

func main() {
	// parse hostname
	users, addrs, err := splitHosts(sshHost)
	if err != nil {
		log.Fatalf("Unable to parse hostnames %s: %s", sshHost, err)
	}

	// read the private key
	pk, err := readPrivateKey(sshKey)
	if err != nil {
		log.Fatalf("Unable to read private key %s: %s", sshKey, err)
	}
	log.Printf("Loaded private key from %s", sshKey)

	// setup a ClientConfig for each hop
	// this might already be gone by the time someone connects, but that does not matter.
	configs := make([]*ssh.ClientConfig, len(addrs))
	for i, user := range users {
		configs[i] = &ssh.ClientConfig{
			User:            user,
			Auth:            []ssh.AuthMethod{pk},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
	}
	go connect(addrs, configs) // to speed up startup

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

			go forward(conn, remoteAddr, addrs, configs)
		}
	}()

	// wait for the end
	<-globalContext.Done()
	log.Print("shutting down")
}

//
// PARSER
//

func splitHosts(hosts string) (users []string, addrs []string, err error) {
	// split into individual strings
	hostSlice := strings.Split(hosts, ",")
	users = make([]string, len(hostSlice))
	addrs = make([]string, len(hostSlice))

	// split each host
	for i, host := range hostSlice {
		users[i], addrs[i], err = splitSingleHost(host)
		if err != nil {
			return nil, nil, err
		}
	}

	return users, addrs, nil
}

func splitSingleHost(host string) (user, addr string, err error) {
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
var clientCloser io.Closer
var clientMutex sync.Mutex

func connect(addrs []string, configs []*ssh.ClientConfig) (*ssh.Client, error) {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// if the client is still alive, return it!
	if clientAlive(client, clientCloser) {
		return client, nil
	}

	// setup a new client, and make sure to return nil on error!
	var err error
	client, clientCloser, err = newClient(addrs, configs)
	if err != nil {
		client = nil
	}
	return client, err
}

func reconnect(addrs []string, configs []*ssh.ClientConfig) (*ssh.Client, error) {
	// make a new connection
	nClient, nClientCloser, err := newClient(addrs, configs)
	if err != nil {
		return nil, err
	}

	// lock the client mutex
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// store it!
	client, clientCloser = nClient, nClientCloser
	return nClient, nil
}

const counterCycle = uint64(1000)

var counter uint64

func scheduleReconnect(addrs []string, configs []*ssh.ClientConfig) {
	if atomic.AddUint64(&counter, 1)%uint64(counterCycle) != 0 {
		return
	}
	log.Println("scheduling reconnect")
	go reconnect(addrs, configs)
}

//
// forwarding
//

var waitPool = &sync.Pool{
	New: func() interface{} {
		return new(sync.WaitGroup)
	},
}

func forward(conn net.Conn, remoteAddr string, addrs []string, configs []*ssh.ClientConfig) {
	defer conn.Close()

	// get or make a new client
	client, err := connect(addrs, configs)
	if err != nil {
		return
	}

	// make a new connection to the remote address
	dest, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("Failed to dial %s, attempting to reconnect: %s", remoteAddr, err)

		// get or make a new client
		client, err := reconnect(addrs, configs)
		if err != nil {
			return
		}

		// make a new connection to the remote address
		dest, err = client.Dial("tcp", remoteAddr)
		if err != nil {
			log.Printf("Failed to dial %s (final): %s", remoteAddr, err)
			return
		}
	}

	// reconnect every couple connections
	go scheduleReconnect(addrs, configs)

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
