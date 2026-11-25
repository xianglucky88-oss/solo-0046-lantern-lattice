package main

import (
	"errors"
	"flag"
	"net"
	"strconv"
)

type config struct{ address, dir string }

func readConfig() (config, error) {
	var c config
	port := flag.Int("port", 8752, "loopback TCP port (0 chooses a free port)")
	flag.StringVar(&c.dir, "data", ".local", "artifact directory")
	flag.Parse()
	if *port < 0 || *port > 65535 || c.dir == "" || flag.NArg() != 0 {
		return c, errors.New("invalid server options")
	}
	c.address = net.JoinHostPort("127.0.0.1", strconv.Itoa(*port))
	return c, nil
}
