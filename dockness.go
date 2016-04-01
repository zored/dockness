package main

import (
	"flag"
	"github.com/miekg/dns"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
)

var ttl uint
var user string

func lookup(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)

	m.SetReply(r)

	var rr dns.RR
	for _, q := range m.Question {
		if q.Qtype != dns.TypeA {
			continue
		}

		domLevels := strings.Split(q.Name, ".")
		domLevelsLen := len(domLevels)
		if domLevelsLen < 3 {
			log.Printf("Couldn't parse the DNS question '%s'", q.Name)
			continue
		}
		machine := domLevels[len(domLevels)-3]

		var output []byte
		var err error
		if user == "" {
			output, err = exec.Command("docker-machine", "ip", machine).CombinedOutput()
		} else {
			output, err = exec.Command("sudo", "-u", user, "docker-machine", "ip", machine).CombinedOutput()
		}

		if err != nil {
			log.Printf("No IP found for machine '%s' (%s)", machine, output)
			continue
		}
		ip := string(output[:len(output)-1])

		rr = &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    uint32(ttl),
			},
			A: net.ParseIP(ip).To4(),
		}

		m.Answer = append(m.Answer, rr)
	}

	w.WriteMsg(m)
}

func main() {
	tld := flag.String("tld", "docker", "Top-level domain to use")
	flag.UintVar(&ttl, "ttl", 0, "Time to Live for DNS records")
	port := flag.String("port", "53", "Port to listen on")
	serverOnly := flag.Bool("server-only", false, "Server only, doesn't try to create a resolver configuration")
	flag.StringVar(&user, "user", os.Getenv("SUDO_USER"), "Execute the 'docker-machine ip' command as this user")
	flag.Parse()

	if *serverOnly == false && runtime.GOOS == "darwin" {
		confPath := "/etc/resolver/" + *tld
		log.Printf("Creating configuration file at %s...", confPath)
		conf := []byte("nameserver 127.0.0.1\nport " + *port + "\n")
		if err := ioutil.WriteFile(confPath, conf, 0644); err != nil {
			log.Fatalf("Could not create configuration file: %s", err)
		}
		defer os.Remove(confPath)
	}

	addr := ":" + *port
	server := &dns.Server{
		Addr: addr,
		Net:  "udp",
	}

	dns.HandleFunc(*tld+".", lookup)

	log.Printf("Listening on %s...", addr)
	go log.Fatal(server.ListenAndServe())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)
	<-sig
}
