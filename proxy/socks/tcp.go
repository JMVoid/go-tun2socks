package socks

import (
	"github.com/eycorsican/go-tun2socks/common"
	"io"
	"net"
	"strconv"
	"sync"

	"golang.org/x/net/proxy"

	"github.com/eycorsican/go-tun2socks/common/log"
	"github.com/eycorsican/go-tun2socks/core"
)

type tcpHandler struct {
	sync.Mutex

	proxyHost string
	proxyPort uint16
	cache     common.Cache
	route     common.Route
}

func NewTCPHandler(proxyHost string, proxyPort uint16, cache common.Cache, route common.Route) core.TCPConnHandler {
	return &tcpHandler{
		proxyHost: proxyHost,
		proxyPort: proxyPort,
		cache:     cache,
		route:     route,
	}
}

type direction byte

const (
	dirUplink direction = iota
	dirDownlink
)

type duplexConn interface {
	net.Conn
	CloseRead() error
	CloseWrite() error
}

func (h *tcpHandler) relay(lhs, rhs net.Conn) {
	upCh := make(chan struct{})

	cls := func(dir direction, interrupt bool) {
		lhsDConn, lhsOk := lhs.(duplexConn)
		rhsDConn, rhsOk := rhs.(duplexConn)
		if !interrupt && lhsOk && rhsOk {
			switch dir {
			case dirUplink:
				lhsDConn.CloseRead()
				rhsDConn.CloseWrite()
			case dirDownlink:
				lhsDConn.CloseWrite()
				rhsDConn.CloseRead()
			default:
				panic("unexpected direction")
			}
		} else {
			lhs.Close()
			rhs.Close()
		}
	}

	// Uplink
	go func() {
		var err error
		_, err = io.Copy(rhs, lhs)
		if err != nil {
			cls(dirUplink, true) // interrupt the conn if the error is not nil (not EOF)
		} else {
			cls(dirUplink, false) // half close uplink direction of the TCP conn if possible
		}
		upCh <- struct{}{}
	}()

	// Downlink
	var err error
	_, err = io.Copy(lhs, rhs)
	if err != nil {
		cls(dirDownlink, true)
	} else {
		cls(dirDownlink, false)
	}

	<-upCh // Wait for uplink done.
}

func (h *tcpHandler) Handle(conn net.Conn, target *net.TCPAddr) error {
	var c net.Conn
	var err error

	dialer, err := proxy.SOCKS5("tcp", core.ParseTCPAddr(h.proxyHost, h.proxyPort).String(), nil, nil)
	if err != nil {
		return err
	}
	targetIP, targetPort := target.IP, target.Port
	host, useFallback, found := h.cache.GetHostByIP(targetIP)
	if found && useFallback {
		filterTarget := net.JoinHostPort(host, strconv.Itoa(targetPort))
		c, err = dialer.Dial(target.Network(), filterTarget)
	} else {
		c, err = dialer.Dial(target.Network(), target.String())
		if h.route.AddToTable() {
			h.route.AddDestWithOrigin(target.String())
		}
	}
	if err != nil {
		return err
	}

	go h.relay(conn, c)

	log.Infof("new proxy connection to %v", target)

	return nil
}
