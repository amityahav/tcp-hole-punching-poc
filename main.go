package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
)

type (
	DialReq struct {
		PeerID        string `json:"peer_id"`
		MyID          string `json:"my_id"`
		MyPrivateAddr string `json:"my_private_addr"`
	}
	DialResp struct {
		PeerID          string `json:"peer_id"`
		PeerPublicAddr  string `json:"peer_public_addr"`
		PeerPrivateAddr string `json:"peer_private_addr"`
	}
	Config struct {
		PublicServerAddress string `json:"public_server_address"`
	}
)

func main() {
	if len(os.Args) != 3 {
		panic("not enough arguments")
	}

	mode := os.Args[1]

	switch mode {
	case "server":
		port := os.Args[2]
		server := RelayServer{
			connMappings: make(map[string]Mapping),
		}

		ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", port))
		if err != nil {
			panic(err)
		}

		server.Serve(ln)
	default:
		var (
			req  DialReq
			peer string
		)

		switch mode {
		case "peer_A":
			peer = "peer_B"
			req = DialReq{
				PeerID: peer,
				MyID:   mode,
			}
		case "peer_B":
			peer = "peer_A"
			req = DialReq{
				PeerID: peer,
				MyID:   mode,
			}
		default:
			panic("unknown mode")
		}

		var cfg Config
		cfgPath := os.Args[2]
		f, err := os.Open(cfgPath)
		if err != nil {
			log.Fatal(err)
		}

		err = json.NewDecoder(f).Decode(&cfg)
		if err != nil {
			log.Fatal(err)
		}

		_ = f.Close()

		conn, err := Dial(cfg, req)
		if err != nil {
			panic(err)
		}

		defer conn.Close()

		go func() {
			reader := bufio.NewReader(os.Stdin)
			for {
				text, _ := reader.ReadString('\n')

				_, err = conn.Write([]byte(text))
				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
						return
					}

					panic(err)
				}
			}
		}()

		go func() {
			for {
				buff := make([]byte, 1024)
				n, err := conn.Read(buff)
				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
						return
					}

					panic(err)
				}

				fmt.Printf("%s> %s", peer, string(buff[:n]))
			}
		}()

		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt)
		<-signalCh
		close(signalCh)
	}
}
