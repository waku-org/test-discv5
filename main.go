package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/dnsdisc"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/waku-org/go-discover/discover"
	wenr "github.com/waku-org/go-waku/waku/v2/protocol/enr"
	"github.com/waku-org/go-waku/waku/v2/protocol/filter"
	"github.com/waku-org/go-waku/waku/v2/protocol/lightpush"
	"github.com/waku-org/go-waku/waku/v2/protocol/relay"
	"github.com/waku-org/go-waku/waku/v2/protocol/store"
)

func main() {
	var portFlag = flag.Int("port", 6000, "port number")
	var bootnodeFlag = flag.String("bootnodes", "", "comma separated bootnodes")
	var dnsdiscFlag = flag.String("dns-disc-url", "", "DNS discovery URL")

	flag.Parse()

	db, err := enode.OpenDB("")
	if err != nil {
		panic(err)
	}

	ip := GetLocalIP()

	priv, _ := crypto.GenerateKey()
	localnode := enode.NewLocalNode(db, priv)
	localnode.SetFallbackIP(ip)

	var protocolID = [6]byte{'d', '5', 'w', 'a', 'k', 'u'}

	bootnodesStr := strings.Split(*bootnodeFlag, ",")
	var bootnodes []*enode.Node

	for _, addr := range bootnodesStr {
		if addr == "" {
			continue
		}
		bootnode, err := enode.Parse(enode.ValidSchemes, addr)
		if err != nil {
			panic(err)
		}
		bootnodes = append(bootnodes, bootnode)
	}

	if *dnsdiscFlag != "" {
		c := dnsdisc.NewClient(dnsdisc.Config{})
		tree, err := c.SyncTree(*dnsdiscFlag)
		if err != nil {
			panic(err)
		}

		bootnodes = append(bootnodes, tree.Nodes()...)
	}

	config := discover.Config{
		PrivateKey: priv,
		Bootnodes:  bootnodes,
		V5Config: discover.V5Config{
			ProtocolID: &protocolID,
		},
	}

	udpAddr := &net.UDPAddr{
		IP:   ip,
		Port: *portFlag,
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(err)
	}

	udpAddr = conn.LocalAddr().(*net.UDPAddr)
	ctx, cancel := context.WithCancel(context.Background())
	if !udpAddr.IP.IsLoopback() {
		go func() {
			nat.Map(nat.Any(), ctx.Done(), "udp", udpAddr.Port, udpAddr.Port, "test discv5 discovery")
		}()
	}

	localnode.SetFallbackUDP(udpAddr.Port)

	fmt.Println("Your node:")
	fmt.Println(localnode.Node())
	listener, err := discover.ListenV5(conn, localnode, config)
	if err != nil {
		panic(err)
	}

	if len(bootnodes) > 0 {
		fmt.Println("\nBootnodes:")
		for i, b := range bootnodes {
			fmt.Println(i+1, "-", b.String())
		}
	}

	peerDelay := 50 * time.Millisecond
	bucketSize := 15

	fmt.Println("\nDiscovered peers:")
	fmt.Println("===============================================================================")

	go func() {
		iterator := listener.RandomNodes()
		seen := make(map[enode.ID]*enode.Node)
		peerCnt := 0
		for iterator.Next() {

			start := time.Now()
			hasNext := iterator.Next()
			if !hasNext {
				break
			}
			elapsed := time.Since(start)

			if elapsed < peerDelay {
				t := time.NewTimer(peerDelay - elapsed)
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					t.Stop()
				}
			}

			// Delay every 15 peers being returned
			peerCnt++
			if peerCnt == bucketSize {
				peerCnt = 0
				t := time.NewTimer(5 * time.Second)
				fmt.Println("Waiting 5 secs...")
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					t.Stop()
					fmt.Println("Attempting to discover more peers")
				}
			}

			node := iterator.Node()
			_ = node
			n, ok := seen[node.ID()]

			recType := "NEW"

			if ok {
				if node.Seq() > n.Seq() {
					seen[node.ID()] = node
					recType = "UPDATE"
				} else {
					continue
				}
			} else {
				seen[node.ID()] = node
			}

			fmt.Println(len(seen), "-", recType, "-", node.String())

			peerID, multiaddresses, err := wenr.Multiaddress(node)
			if err == nil {
				fmt.Println("peerID", peerID)
				if len(multiaddresses) > 0 {
					fmt.Println("multiaddr:", multiaddresses)
				} else {
					fmt.Println("multiaddr: field contains no value")
				}
			} else {
				fmt.Println("multiaddr:", err.Error())
			}

			fmt.Println(fmt.Sprintf("ip %s:%d", node.IP(), node.TCP()))
			shards, err := wenr.RelaySharding(node.Record())
			if err != nil {
				fmt.Println("rs/rsv:", "field contains no value")
			}
			if shards != nil {
				fmt.Println("cluster-id: ", shards.ClusterID)
				fmt.Println("shards: ", shards.ShardIDs)
			} else {
				fmt.Println("cluster-id:", "not available")
				fmt.Println("shards:", "not available")
			}
			DecodeWaku2ENRField(node.Record())
			fmt.Println()

			select {
			case <-ctx.Done():
				return
			default:
			}
		}
		iterator.Close()

		select {
		case <-ctx.Done():
			return
		default:
		}
	}()

	// Wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("\n\n\nshutting down...")

	cancel()

}

func ReadValue(record *enr.Record, name string) ([]byte, error) {
	var field []byte
	if err := record.Load(enr.WithEntry(name, &field)); err != nil {
		if enr.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return field, nil
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return net.IPv4zero
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP
			}
		}
	}
	return net.IPv4zero
}

func DecodeWaku2ENRField(record *enr.Record) {
	//Decoding Waku2 field
	var enrField wenr.WakuEnrBitfield
	var protosSupported []string
	if err := record.Load(enr.WithEntry("waku2", &enrField)); err != nil {
		if enr.IsNotFound(err) {
			fmt.Println("waku2:", "field contains no value")
		} else {
			panic(err)
		}
	}

	if enrField&relay.WakuRelayENRField != 0 {
		protosSupported = append(protosSupported, string(relay.WakuRelayID_v200))
	}
	if enrField&filter.FilterSubscribeENRField != 0 {
		protosSupported = append(protosSupported, string(filter.FilterSubscribeID_v20beta1))
	}
	if enrField&lightpush.LightPushENRField != 0 {
		protosSupported = append(protosSupported, string(lightpush.LightPushID_v20beta1))
	}
	if enrField&store.StoreENRField != 0 {
		protosSupported = append(protosSupported, string(store.StoreID_v20beta4))
	}
	fmt.Println("Wakuv2 Protocols Supported:")
	for _, proto := range protosSupported {
		fmt.Println(proto)
	}
}
