package main

import (
	"log"
	"net"
	"net/rpc"

	"ds-integral.com/master/api"
	"ds-integral.com/master/config"
	"ds-integral.com/master/icalc"
	"ds-integral.com/master/shared"
)

const masterIP = "172.20.1.158"
const rpcPort = ":3410"
const apiPort = ":8080"

func main() {
	log.Println("Starting Integral Master...")

	// Load configuration
	config.Load()

	// Create calculator
	calc := icalc.NewCalc()

	// Create RPC server
	calcRPC := shared.NewCalcRPC(calc)
	err := rpc.Register(calcRPC)
	if err != nil {
		log.Fatalf("Failed to register RPC: %s", err)
	}

	// Start RPC server for workers
	rpcAddr := masterIP + rpcPort
	listener, err := net.Listen("tcp", rpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %s", rpcAddr, err)
	}

	log.Printf("RPC Server listening on %s (for workers)", rpcAddr)

	// Start RPC server in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Failed to accept connection: %s", err)
				continue
			}
			go rpc.ServeConn(conn)
		}
	}()

	// Start HTTP API server
	apiAddr := masterIP + apiPort
	apiServer := api.NewServer(calc)
	log.Printf("Starting HTTP API Server on %s", apiAddr)
	log.Fatal(apiServer.Start(apiAddr))
}
