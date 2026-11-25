package main

import (
	"example.local/lantern-lattice/internal/service"
	"log"
	"net"
	"net/http"
	"time"
)

func main() {
	c, err := readConfig()
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", c.address)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Handler: service.New(c.dir).Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Printf("listening %s", listener.Addr())
	log.Fatal(server.Serve(listener))
}
