package rndz

import (
	"context"
	"net"
	"net/netip"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"

	logging "github.com/ipfs/go-log/v2"
	ma "github.com/multiformats/go-multiaddr"
	mafmt "github.com/multiformats/go-multiaddr-fmt"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/optman/rndz-go/client/tcp"
)

var log = logging.Logger("rndz-tcp-tpt")

type Option func(*RndzTransport) error

func WithId(id peer.ID) Option {
	return func(t *RndzTransport) error {
		t.Id = id
		return nil
	}
}

func WithRndzServer(addr string) Option {
	return func(t *RndzTransport) (err error) {
		t.RndzServer, err = ma.NewMultiaddr(addr)
		return
	}
}

type RndzTransport struct {
	Upgrader   transport.Upgrader
	rcmgr      network.ResourceManager
	RndzServer ma.Multiaddr
	Id         peer.ID
}

var _ transport.Transport = &RndzTransport{}

func NewRNDZTransport(upgrader transport.Upgrader, rcmgr network.ResourceManager, opts ...Option) (*RndzTransport, error) {
	if rcmgr == nil {
		rcmgr = network.NullResourceManager
	}

	tr := &RndzTransport{
		Upgrader: upgrader,
		rcmgr:    rcmgr,
	}
	for _, o := range opts {
		if err := o(tr); err != nil {
			return nil, err
		}
	}
	return tr, nil
}

var dialMatcher = mafmt.TCP

func (t *RndzTransport) CanDial(addr ma.Multiaddr) bool {
	log.Debugf("can dail %s", addr)
	return dialMatcher.Matches(addr)
}

func (t *RndzTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	log.Debugf("dail %s  %s", raddr, p)

	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true)
	if err != nil {
		log.Debugw("resource manager blocked outgoing connection", "peer", p, "addr", raddr, "error", err)
		return nil, err
	}
	if err := connScope.SetPeer(p); err != nil {
		log.Debugw("resource manager blocked outgoing connection for peer", "peer", p, "addr", raddr, "error", err)
		connScope.Done()
		return nil, err
	}

	rndzServerAddr, err := manet.ToNetAddr(raddr)
	if err != nil {
		return nil, err
	}
	log.Debugf("rndz server %s", rndzServerAddr)

	c := tcp.New(rndzServerAddr.String(), t.Id.String(), netip.AddrPort{})
	defer c.Close()
	conn, err := c.Connect(ctx, p.String())
	if err != nil {
		return nil, err
	}

	maConn, err := manet.WrapNetConn(conn)
	if err != nil {
		return nil, err
	}

	return t.Upgrader.Upgrade(ctx, t, maConn, network.DirOutbound, p, connScope)
}

func (t *RndzTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {

	listenAddr, err := manet.ToNetAddr(laddr)
	if err != nil {
		return nil, err
	}

	log.Debugf("listen on %s", listenAddr)
	log.Debugf("rndz server %s", t.RndzServer)

	rndzServer, err := manet.ToNetAddr(t.RndzServer)
	if err != nil {
		return nil, err
	}

	//NOTE: rndzServer and listenAddr must be the same ip family
	c := tcp.New(rndzServer.String(), t.Id.String(), listenAddr.(*net.TCPAddr).AddrPort())
	listener, err := c.Listen(context.Background())
	if err != nil {
		return nil, err
	}

	maListener, err := manet.WrapNetListener(listener)
	if err != nil {
		return nil, err
	}

	return t.Upgrader.UpgradeListener(t, maListener), nil
}

func (t *RndzTransport) Protocols() []int {
	return []int{ma.P_TCP}
}

func (t *RndzTransport) Proxy() bool {
	return false
}

func (t *RndzTransport) String() string {
	return "RNDZ-TCP"
}
