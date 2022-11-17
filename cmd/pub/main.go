package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/yofu/rz2"
)

var endian = binary.BigEndian

var (
	defaultconfig   = &rz2.Config{
		Server: "tcp://192.168.100.148:1883",
		Cafile: "",
		Crtfile: "",
		Keyfile: "",
		Homedir: os.Getenv("HOME"),
		Removehour: 0,
		List: make([]string, 0),
	}
)

func StartSubscriber(server string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(server)
	opts.SetAutoReconnect(false)
	opts.SetClientID(fmt.Sprintf("rz2rec_%d", time.Now().UnixNano()))
	if strings.HasPrefix(server, "ssl") {
		tlsconfig, err := rz2.NewTLSConfig(defaultconfig.Cafile, defaultconfig.Crtfile, defaultconfig.Keyfile)
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tlsconfig)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		log.Printf("not connected: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		return client, token.Error()
	}
	log.Printf("connected to %s: %s\n", server, time.Now().Format("2006-01-02 15:04:05"))

	return client, nil
}

func main() {
	conffn := flag.String("config", "", "config file")
	flag.Parse()

	if *conffn != "" {
		err := defaultconfig.ReadConfig(*conffn)
		if err != nil {
			log.Fatal(err)
		}
	}

	defaultconfig.Println()

	basedir := filepath.Join(defaultconfig.Homedir, "rz2/recorder/log")
	os.MkdirAll(basedir, 0755)
	logfile, err := os.Create(filepath.Join(basedir, fmt.Sprintf("log%s.0", time.Now().Format("20060102150405"))))
	if err != nil {
		log.Fatal(err)
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	srvaddress, err := rz2.ServerAddress(defaultconfig.Server)
	if srvaddress == "" {
		log.Fatal(err)
	}
	client, err := StartSubscriber(srvaddress)

	ticker := time.NewTicker(time.Second)
	conticker := time.NewTicker(time.Second)
	now := time.Now().UnixMilli()
	topic := fmt.Sprintf("b8:27:eb:%02d:%02d:%02d", now%2, now%3, now%5)
	message := "test"
	for {
		select {
		case <-ticker.C:
			token := client.Publish(topic, 0, false, message)
			fmt.Println(topic, message)
			token.Wait()
		case <-conticker.C:
			if !client.IsConnected() {
				log.Printf("not connected: %s", time.Now().Format("2006-01-02 15:04:05"))
				for {
					if client.IsConnected() {
						log.Printf("reconnected: %s", time.Now().Format("2006-01-02 15:04:05"))
						for _, t := range defaultconfig.List {
							client.Subscribe(t, 0, nil)
						}
						break
					}
					token := client.Connect()
					token.WaitTimeout(time.Second * 5)
				}
			}
		}
	}
}

