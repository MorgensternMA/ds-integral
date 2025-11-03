package main

import (
	"flag"
	"log"
	"net"

	"ds-integral.com/worker/calculator"
)

func main() {
	log.Println("Starting Integral Worker...")

	var masterAddr string
	var masterPort int
	flag.StringVar(&masterAddr, "master", "172.20.1.158", "Master IP address")
	flag.IntVar(&masterPort, "port", 3410, "Master port")
	flag.Parse()

	ip := net.ParseIP(masterAddr)
	if ip == nil {
		log.Fatalf("Invalid master IP address: %s", masterAddr)
	}

	log.Printf("Connecting to master at %s:%d", masterAddr, masterPort)

	calc := calculator.NewCalculator(ip, masterPort)
	calc.Run()
}
