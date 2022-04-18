package tcp

import (
	"context"
	"errors"
	"net/netip"

	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	tpt "github.com/libp2p/go-libp2p-core/transport"

	logging "github.com/ipfs/go-log/v2"
	ma "github.com/multiformats/go-multiaddr"
	mafmt "github.com/multiformats/go-multiaddr-fmt"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/optman/rndz-go/client/tcp"
	ra "github.com/optman/rndz-multiaddr"
)

var log = logging.Logger("rndz-tcp-tpt")

type Option func(*transport) error

type transport struct {
	upgrader tpt.Upgrader
	rcmgr    network.ResourceManager
	peerId   peer.ID
}

var InvalidListenAddr = errors.New("invalid listen addr")

var _ tpt.Transport = &transport{}

func NewTransport(key ic.PrivKey, upgrader tpt.Upgrader, rcmgr network.ResourceManager, opts ...Option) (*transport, error) {
	if rcmgr == nil {
		rcmgr = network.NullResourceManager
	}

	peerId, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}

	tr := &transport{
		upgrader: upgrader,
		rcmgr:    rcmgr,
		peerId:   peerId,
	}
	for _, o := range opts {
		if err := o(tr); err != nil {
			return nil, err
		}
	}
	return tr, nil
}

var dialMatcher = mafmt.TCP

func (t *transport) CanDial(addr ma.Multiaddr) bool {
	return dialMatcher.Matches(addr)
}

func (t *transport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (tpt.CapableConn, error) {
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

	c := tcp.New(rndzServerAddr.String(), t.peerId.String(), netip.AddrPort{})
	defer c.Close()
	conn, err := c.Connect(ctx, p.String())
	if err != nil {
		return nil, err
	}

	maConn, err := manet.WrapNetConn(conn)
	if err != nil {
		return nil, err
	}

	return t.upgrader.Upgrade(ctx, t, maConn, network.DirOutbound, p, connScope)
}

func (t *transport) Listen(addr ma.Multiaddr) (tpt.Listener, error) {
	log.Debugf("Listen %s", addr)

	localAddr, rndzAddr := ra.SplitListenAddr(addr)
	if rndzAddr == nil {
		return nil, InvalidListenAddr
	}

	laddr, err := manet.ToNetAddr(localAddr)
	if err != nil {
		return nil, InvalidListenAddr
	}

	raddr, err := manet.ToNetAddr(rndzAddr)
	if err != nil {
		return nil, InvalidListenAddr
	}

	rndz := tcp.New(raddr.String(), t.peerId.String(), netip.MustParseAddrPort(laddr.String()))
	listener, err := rndz.Listen(context.Background())
	if err != nil {
		return nil, err
	}

	maListener, err := manet.WrapNetListener(listener)
	if err != nil {
		return nil, err
	}

	return t.upgrader.UpgradeListener(t, maListener), nil
}

func (t *transport) Protocols() []int {
	return []int{ma.P_TCP}
}

func (t *transport) Proxy() bool {
	return false
}

func (t *transport) String() string {
	return "RNDZ-TCP"
}
