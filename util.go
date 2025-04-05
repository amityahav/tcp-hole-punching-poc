package main

import (
	"fmt"
	"net"
	"strconv"
)

func ipPortToBytes(ipPortStr string) ([4]byte, int, error) {
	host, portStr, err := net.SplitHostPort(ipPortStr)
	if err != nil {
		return [4]byte{}, 0, fmt.Errorf("invalid IP:port format")
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return [4]byte{}, 0, fmt.Errorf("invalid IP address")
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return [4]byte{}, 0, fmt.Errorf("not an IPv4 address")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return [4]byte{}, 0, fmt.Errorf("invalid port number")
	}

	var ipBytes [4]byte
	copy(ipBytes[:], ip4)

	return ipBytes, port, nil
}
