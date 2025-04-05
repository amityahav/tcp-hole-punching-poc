package main

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"
)

type (
	Mapping struct {
		PublicAddr  string
		PrivateAddr string
		Conn        net.Conn
	}
	RelayServer struct {
		mu           sync.RWMutex
		connMappings map[string]Mapping
	}
)

func (s *RelayServer) Serve(ln net.Listener) {
	log.Println("Listening on", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}

		go func() {
			err := s.handleConn(conn)
			if err != nil {
				log.Println(err)
				_ = conn.Close()
			}
		}()
	}
}

func (s *RelayServer) handleConn(conn net.Conn) error {
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var dr DialReq
	err := dec.Decode(&dr)
	if err != nil {
		return err
	}

	newMapping := Mapping{
		PublicAddr:  conn.RemoteAddr().String(),
		PrivateAddr: dr.MyPrivateAddr,
		Conn:        conn,
	}

	log.Printf("new dial request: to %s, from %s, private_addr %s, public_addr %s", dr.PeerID, dr.MyID, dr.MyPrivateAddr, newMapping.PublicAddr)

	// save connections addresses
	s.mu.RLock()
	s.connMappings[dr.MyID] = newMapping
	s.mu.RUnlock()

	// see if there's a mapping for peer
	var peer Mapping
	for {
		p, ok := s.connMappings[dr.PeerID]
		if !ok {
			time.Sleep(1 * time.Second)
			continue
		}

		peer = p
		log.Printf("peer %s connected: public_address: %s, private_address: %s", dr.PeerID, peer.PublicAddr, p.PrivateAddr)

		break
	}

	// send conn the peer's addresses
	err = enc.Encode(DialResp{
		PeerID:          dr.PeerID,
		PeerPublicAddr:  peer.PublicAddr,
		PeerPrivateAddr: peer.PrivateAddr,
	})
	if err != nil {
		return err
	}

	// send peer conn's addresses
	peerEnc := json.NewEncoder(peer.Conn)
	err = peerEnc.Encode(DialResp{
		PeerID:          dr.MyID,
		PeerPublicAddr:  newMapping.PublicAddr,
		PeerPrivateAddr: newMapping.PrivateAddr,
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.connMappings, dr.PeerID)
	s.mu.Unlock()

	return nil
}
