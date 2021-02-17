package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/yofu/rz2"
)

var pattern = regexp.MustCompile("^[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}.dat$")

var (
	cafile   = ""
	crtfile  = ""
	keyfile  = ""
	homedir  = os.Getenv("HOME")
	recorder *rz2.Recorder
	recdir   = ""
)

func removeoldfiles(dur time.Duration) error {
	rd, err := ioutil.ReadDir(recdir)
	if err != nil {
		return err
	}

	t0 := time.Now().Add(dur)

	for _, d := range rd {
		if d.IsDir() {
			continue
		}
		if !pattern.MatchString(d.Name()) {
			continue
		}
		if d.ModTime().Before(t0) {
			os.Remove(filepath.Join(recdir, d.Name()))
		}
	}
	return nil
}

func StartSubscriber(server string, topics []string, fn func(mqtt.Client, mqtt.Message)) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(server)
	opts.SetAutoReconnect(false)
	opts.SetClientID(fmt.Sprintf("rz2rec_%d", time.Now().UnixNano()))
	if strings.HasPrefix(server, "ssl") {
		tlsconfig, err := rz2.NewTLSConfig(cafile, crtfile, keyfile)
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tlsconfig)
	}

	opts.SetDefaultPublishHandler(fn)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		log.Printf("not connected: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		return client, token.Error()
	}
	log.Printf("connected to %s: %s\n", server, time.Now().Format("2006-01-02 15:04:05"))

	for _, t := range topics {
		client.Subscribe(t, 0, nil)
	}

	return client, nil
}

func main() {
	server := flag.String("server", "tcp://192.168.1.23:18884", "server url:port")
	cafn := flag.String("cafile", "", "ca file")
	crtfn := flag.String("crtfile", "", "crt file")
	keyfn := flag.String("keyfile", "", "key file")
	home := flag.String("home", "", "home directory")
	directory := flag.String("dir", "", "save directory")
	flag.Parse()

	if *cafn != "" {
		cafile = *cafn
	}
	if *crtfn != "" {
		crtfile = *crtfn
	}
	if *keyfn != "" {
		keyfile = *keyfn
	}
	if *home != "" {
		homedir = *home
	}

	basedir := filepath.Join(homedir, "rz2/recorder/log")
	os.MkdirAll(basedir, 0755)
	logfile, err := os.Create(filepath.Join(basedir, fmt.Sprintf("log%s.0", time.Now().Format("20060102150405"))))
	if err != nil {
		log.Fatal(err)
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	if *directory == "" {
		recdir = filepath.Join(homedir, "rz2/recorder")
	} else {
		recdir = *directory
	}
	dest, err := rz2.TimeStampDest(recdir)
	if err != nil {
		log.Fatal(err)
	}
	recorder = rz2.NewRecorder(dest)

	client, err := StartSubscriber(*server, []string{"#"}, func(client mqtt.Client, msg mqtt.Message) {
		err := recorder.Record(msg)
		if err != nil {
			log.Println(err)
		}
	})

	ticker := time.NewTicker(time.Minute * 60)
	conticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			recorder.SetDest(dest)
			err := removeoldfiles(-365 * 24 * time.Hour) // 365 days before
			if err != nil {
				log.Println(err)
			}
		case <-conticker.C:
			if !client.IsConnected() {
				log.Printf("not connected: %s", time.Now().Format("2006-01-02 15:04:05"))
				for {
					if client.IsConnected() {
						log.Printf("reconnected: %s", time.Now().Format("2006-01-02 15:04:05"))
						client.Subscribe("#", 0, nil)
						break
					}
					token := client.Connect()
					token.WaitTimeout(time.Second * 5)
				}
			}
		}
	}
}
