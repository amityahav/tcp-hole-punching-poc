package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"net"
	"os"
	"sync/atomic"
	"syscall"
)

func Dial(cfg Config, req DialReq) (net.Conn, error) {
	peerConCh := make(chan net.Conn, 1)
	var dialed atomic.Bool

	serverConn, err := net.Dial("tcp", cfg.PublicServerAddress)
	if err != nil {
		return nil, err
	}

	myAddress := serverConn.LocalAddr().String()
	rawServerConn, err := serverConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		return nil, err
	}

	req.MyPrivateAddr = myAddress

	log.Printf("dailed to server: %s -> %s", myAddress, serverConn.RemoteAddr().String())

	var innerErr error
	err = rawServerConn.Control(func(fd uintptr) {
		innerErr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if innerErr != nil {
			return
		}

		innerErr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if innerErr != nil {
			return
		}
	})
	if err != nil {
		return nil, err
	}

	if innerErr != nil {
		return nil, innerErr
	}

	//go func() {
	//	for {
	//		if dialed.Load() {
	//			return
	//		}
	//
	//		acceptingSocket, err := newReusableSocket(myAddress)
	//		if err != nil {
	//			panic(err)
	//		}
	//
	//		err = unix.Listen(acceptingSocket, 1024)
	//		if err != nil {
	//			log.Println(err)
	//			continue
	//		}
	//
	//		nfd, _, err := unix.Accept(acceptingSocket)
	//		if err != nil {
	//			log.Println(err)
	//			continue
	//		}
	//
	//		peerFile := os.NewFile(uintptr(nfd), fmt.Sprintf("/dev/tcp_socket_%s_6", req.MyID))
	//		peerConn, err := net.FileConn(peerFile)
	//		if err != nil {
	//			_ = syscall.Close(nfd)
	//			log.Println("dial public:", err)
	//			continue
	//		}
	//
	//		select {
	//		case peerConCh <- peerConn:
	//			log.Printf("accepted new connection from peer: %s -> %s", peerConn.LocalAddr(), peerConn.RemoteAddr())
	//			return
	//		default:
	//			return
	//		}
	//	}
	//}()

	enc := json.NewEncoder(serverConn)
	dec := json.NewDecoder(serverConn)

	// send the dial request to the server
	err = enc.Encode(req)
	if err != nil {
		return nil, err
	}

	// wait for peers addresses from the server
	var res DialResp
	err = dec.Decode(&res)
	if err != nil {
		return nil, err
	}

	log.Printf("recieved peer info: id %s, public_addr %s, private_addr %s", res.PeerID, res.PeerPublicAddr, res.PeerPrivateAddr)

	// try dialing public/private addresses until one of them "locks in" (tcp simultaneous open)
	// NOTE: should be done with the same local ip:port

	// dial public endpoint
	go func() {
		for {
			if dialed.Load() {
				return
			}

			publicFd, err := newReusableSocket(myAddress)
			if err != nil {
				log.Println("failed creating reusable socket (PUBLIC):", err)
				continue
			}

			peerPublicAddr, peerPublicPort, err := ipPortToBytes(res.PeerPublicAddr)
			if err != nil {
				_ = syscall.Close(publicFd)
				log.Println("failed fetching ip:port (PUBLIC):", err)
				continue
			}

			err = syscall.Connect(publicFd, &syscall.SockaddrInet4{
				Port: peerPublicPort,
				Addr: peerPublicAddr,
			})
			if err != nil {
				_ = syscall.Close(publicFd)
				log.Printf("raw connect to peer %s (PUBLIC): %s", res.PeerPublicAddr, err)
				if err.Error() == "address already in use" {
					return
				}

				continue
			}

			peerFile := os.NewFile(uintptr(publicFd), fmt.Sprintf("/dev/tcp_socket_%s_3", req.MyID))
			peerConn, err := net.FileConn(peerFile)
			if err != nil {
				_ = syscall.Close(publicFd)
				log.Println("dial public:", err)
				continue
			}

			select {
			case peerConCh <- peerConn:
				log.Printf("dialed new connection to peer (PUBLIC): %s -> %s", peerConn.LocalAddr(), peerConn.RemoteAddr())
				return
			default:
				return
			}
		}
	}()

	// dial private endpoint
	go func() {
		for {
			if dialed.Load() {
				return
			}

			privateFd, err := newReusableSocket(myAddress)
			if err != nil {
				log.Println("failed creating reusable socket (PRIVATE):", err)
				continue
			}

			peerPrivateAddr, peerPrivatePort, err := ipPortToBytes(res.PeerPrivateAddr)
			if err != nil {
				_ = syscall.Close(privateFd)
				log.Println("failed fetching ip:port (PRIVATE):", err)
				continue
			}

			err = syscall.Connect(privateFd, &syscall.SockaddrInet4{
				Port: peerPrivatePort,
				Addr: peerPrivateAddr,
			})
			if err != nil {
				_ = syscall.Close(privateFd)
				log.Printf("raw connect to peer %s (PRIVATE): %s", res.PeerPrivateAddr, err)
				if err.Error() == "address already in use" {
					return
				}

				continue
			}

			peerFile := os.NewFile(uintptr(privateFd), fmt.Sprintf("/dev/tcp_socket_%s_4", req.MyID))
			peerConn, err := net.FileConn(peerFile)
			if err != nil {
				_ = syscall.Close(privateFd)
				log.Println("dial private:", err)
				continue
			}

			select {
			case peerConCh <- peerConn:
				log.Printf("dialed new connection to peer (PRIVATE): %s -> %s", peerConn.LocalAddr(), peerConn.RemoteAddr())
				return
			default:
				return
			}
		}
	}()

	peerConn := <-peerConCh
	dialed.Store(true)

	return peerConn, nil
}

func newReusableSocket(addr string) (int, error) {
	fd, err := syscall.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		return 0, err
	}

	err = syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err != nil {
		return 0, err
	}

	err = syscall.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	if err != nil {
		return 0, err
	}

	myAddr, myPort, err := ipPortToBytes(addr)
	if err != nil {
		return 0, err
	}

	// binding the socket to a fixed addr:port tuple.
	err = syscall.Bind(fd, &syscall.SockaddrInet4{
		Port: myPort,
		Addr: myAddr,
	})
	if err != nil {
		return 0, err
	}

	return fd, nil
}
