package shared

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/exp/rand"
)

const PING_PORT = 8989
const ICALC_PORT = 9999
const DISCOVER_PORT = 9933

func GetIPv4() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// Ignore down or loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			// Check if it's an IPv4 address and not a loopback
			if ip.To4() != nil && !ip.IsLoopback() {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("no non-loopback IPv4 address found")
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString generates a random string of the specified length
func RandomString() string {
	seededRand := rand.New(rand.NewSource(uint64(time.Now().UnixNano())))
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}