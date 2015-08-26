package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
)

// Ready to handle full-size UDP datagram or TCP segment in one step
const (
	BUFFER_LIMIT = 2<<16 - 1
)

type Progress struct {
	ra net.Addr
	rBytes uint64
	wBytes uint64
}

/**
 * Launch two read-write goroutines and waits for signal from them
 */
func TransferStreams(con net.Conn) {
	c1 := readAndWrite(con, os.Stdout)
	c2 := readAndWrite(os.Stdin, con)
	select {
	case progress := <-c1:
		log.Printf("Remote connection is closed: %+v\n", progress)
	case progress := <-c2:
		log.Printf("Local program is terminated: %+v\n", progress)
	}
}

/**
 * Launch receive goroutine first, wait for address from it (if needed), launch send goroutine then.
 */
func TransferPackets(con net.Conn) {
	c1 := readAndWrite(con, os.Stdout)
	// If connection hasn't got remote address then wait for it from receiver goroutine
	ra := con.RemoteAddr()
	if ra == nil {
		progress := <-c1
		ra = progress.ra
		log.Println("Connect from", ra)
	}
	c2 := readAndWriteToAddr(os.Stdin, con, ra)
	select {
	case progress := <-c1:
		log.Printf("Remote connection is closed: %+v\n", progress)
	case progress := <-c2:
		log.Printf("Local program is terminated: %+v\n", progress)
	}
}

/**
 * Read from Reader and write to Writer until EOF.
 */
func readAndWrite(r io.Reader, w io.Writer) <-chan Progress {
	return readAndWriteToAddr(r, w, nil)
}

/**
 * Read from Reader and write to Writer until EOF.
 * ra is an address to whom packets must be sent in UDP listen mode.
 */
func readAndWriteToAddr(r io.Reader, w io.Writer, ra net.Addr) <-chan Progress {
	buf := make([]byte, BUFFER_LIMIT)
	c := make(chan Progress)
	rBytes, wBytes := uint64(0), uint64(0)
	go func() {
		defer func() {
			if con, ok := w.(net.Conn); ok {
				con.Close()
				if _, ok := con.(*net.UDPConn); ok {
					log.Printf("Stop receiving flow from %v\n", ra)
				} else {
					log.Printf("Connection from %v is closed\n", con.RemoteAddr())
				}
			}
			c <- Progress{rBytes: rBytes, wBytes: wBytes, ra: ra}
		}()

		for {
			var n int
			var err error

			// Read
			if con, ok := r.(*net.UDPConn); ok {
				var addr net.Addr
				n, addr, err = con.ReadFrom(buf)
				// Inform caller function with remote address once
				// (for UDP in listen mode only)
				if con.RemoteAddr() == nil && ra == nil {
					ra = addr
					c <- Progress{rBytes: rBytes, wBytes: wBytes, ra: ra}
				}
			} else {
				n, err = r.Read(buf)
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("Read error: %s\n", err)
				}
				break
			}
			rBytes += uint64(n)

			// Write
			if con, ok := w.(*net.UDPConn); ok && con.RemoteAddr() == nil {
				// Special case for UDP in listen mode otherwise
				// net.ErrWriteToConnected will be thrown
				n, err = con.WriteTo(buf[0:n], ra)
			} else {
				n, err = w.Write(buf[0:n])
			}
			if err != nil {
				log.Fatalf("Write error: %s\n", err)
			}
			wBytes += uint64(n)

//			c <- Progress{rBytes: rBytes, wBytes: wBytes, ra: ra}
		}
	}()
	return c
}

func main() {
	var host, port, proto string
	var listen bool
	flag.StringVar(&host, "host", "", "Remote host to connect, i.e. 127.0.0.1")
	flag.StringVar(&proto, "proto", "tcp", "TCP/UDP mode")
	flag.BoolVar(&listen, "listen", false, "Listen mode")
	flag.StringVar(&port, "port", ":9999", "Port to listen on or connect to (prepended by colon), i.e. :9999")
	flag.Parse()

	if proto == "tcp" {
		if listen {
			ln, err := net.Listen(proto, port)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Listening on", proto+port)
			con, err := ln.Accept()
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Connect from", con.RemoteAddr())
			TransferStreams(con)
		} else if host != "" {
			con, err := net.Dial(proto, host+port)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Connected to", host+port)
			TransferStreams(con)
		} else {
			flag.Usage()
		}
	} else if proto == "udp" {
		if listen {
			addr, err := net.ResolveUDPAddr(proto, port)
			if err != nil {
				log.Fatalln(err)
			}
			con, err := net.ListenUDP(proto, addr)
			if err != nil {
				log.Fatalln(err)
			}
			log.Println("Listening on", proto+port)
			TransferPackets(con)
		} else if host != "" {
			addr, err := net.ResolveUDPAddr(proto, host+port)
			if err != nil {
				log.Fatalln(err)
			}
			con, err := net.DialUDP(proto, nil, addr)
			if err != nil {
				log.Fatalln(err)
			}
			TransferPackets(con)
		} else {
			flag.Usage()
		}
	}
}
