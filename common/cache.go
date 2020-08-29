package common

import "net"

type Cache interface {
	GetHostByIP(net.IP) (string, bool, bool)
}
