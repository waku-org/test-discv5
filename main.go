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
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/nat"
	"github.com/waku-org/go-discover/discover"
)

func main() {
	var portFlag = flag.Int("port", 6000, "port number")
	//var privKeyFlag = flag.String("privk", "", "private key")
	var bootnodeFlag = flag.String("bootnodes", "", "comma separated bootnodes")
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

	config := discover.Config{
		PrivateKey:   priv,
		Bootnodes:    bootnodes,
		V5ProtocolID: &protocolID,
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

	fmt.Println(localnode.Node())

	listener, err := discover.ListenV5(conn, localnode, config)
	if err != nil {
		panic(err)
	}

	peerDelay := 100 * time.Millisecond
	bucketSize := 15

	go func() {
		iterator := listener.RandomNodes()
		// seen := make(map[enode.ID]struct{})
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
				}
			}

			node := iterator.Node()
			_ = node
			fmt.Println("Found node...")
			/*_, ok := seen[node.ID()]
			if ok {
				continue
			}
			seen[node.ID()] = struct{}{}

			fmt.Printf(fmt.Sprintf("found %d nodes \n", len(seen)))

			select {
			case <-ctx.Done():
				return
			default:
			}*/
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
