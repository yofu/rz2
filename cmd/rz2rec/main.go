package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/yofu/rz2"
)

var endian = binary.BigEndian

type Config struct {
	Server string `toml:"server"`
	Cafile string `toml:"cafile"`
	Crtfile string `toml:"crtfile"`
	Keyfile string `toml:"keyfile"`
	Homedir string `toml:"homedir"`
	Removehour int `toml:"removehour"`
	List []string `toml:"list"`
}

func (c *Config) Println() {
	fmt.Printf("server: %s\n", c.Server)
	fmt.Printf("cafile: %s\n", c.Cafile)
	fmt.Printf("crtfile: %s\n", c.Crtfile)
	fmt.Printf("keyfile: %s\n", c.Keyfile)
	fmt.Printf("homedir: %s\n", c.Homedir)
	fmt.Printf("removehour: %d\n", c.Removehour)
	fmt.Print("list:\n")
	for i, t := range c.List {
		fmt.Printf("    %d: %s\n", i, t)
	}
	fmt.Println("")
}

var (
	defaultconfig   = &Config{
		Server: "tcp://133.11.95.82:18884",
		Cafile: "",
		Crtfile: "",
		Keyfile: "",
		Homedir: os.Getenv("HOME"),
		Removehour: 0,
		List: make([]string, 0),
	}
)

type Record struct {
	sync.Mutex
	dest       *os.File
	dir        string
	macaddress string
}

func NewRecord(dir, macaddress string) *Record {
	r := new(Record)
	r.dir = dir
	r.macaddress = macaddress
	return r
}

func (r *Record) setdest() error {
	r.Lock()
	if r.dest != nil {
		r.dest.Close()
	}
	now := time.Now()
	name := strings.Replace(r.macaddress, ":", "_", -1)
	w, err := os.Create(filepath.Join(r.dir, fmt.Sprintf("%s_%s.dat", name, now.Format("2006-01-02-15-04-05"))))
	if err != nil {
		return err
	}
	r.dest = w
	r.Unlock()
	return nil
}

func (r *Record) record(msg mqtt.Message) error {
	r.Lock()
	buf := new(bytes.Buffer)
	buf.Grow(8 + len([]byte(msg.Topic())) + 1 + 4 + len(msg.Payload()))
	// Current Time [ms]: 8byte
	ct := time.Now().UnixNano() / 1000000 // ms
	err := binary.Write(buf, endian, ct)
	if err != nil {
		return err
	}
	// Topic
	_, err = buf.WriteString(msg.Topic())
	if err != nil {
		return err
	}
	err = buf.WriteByte(byte(0))
	if err != nil {
		return err
	}
	// Data size: 4byte
	err = binary.Write(buf, endian, int32(len(msg.Payload())))
	if err != nil {
		return err
	}
	// Data
	_, err = buf.Write(msg.Payload())
	if err != nil {
		return err
	}
	_, err = buf.WriteTo(r.dest)
	r.dest.Sync()
	r.Unlock()
	return err
}

func removeoldfiles(dur time.Duration) error {
	dir := filepath.Join(defaultconfig.Homedir, "rz2/recorder")

	rd, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	t0 := time.Now().Add(dur)

	for _, d := range rd {
		if !d.IsDir() {
			continue
		}
		rd2, err := ioutil.ReadDir(filepath.Join(dir, d.Name()))
		if err != nil {
			log.Println(err)
			continue
		}
		for _, d2 := range rd2 {
			if d2.IsDir() {
				continue
			}
			if d2.ModTime().Before(t0) {
				os.Remove(filepath.Join(dir, d.Name(), d2.Name()))
			}
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
		tlsconfig, err := rz2.NewTLSConfig(defaultconfig.Cafile, defaultconfig.Crtfile, defaultconfig.Keyfile)
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

func ReadConfig(fn string) error {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}
	fmt.Println(b)
	fmt.Println(defaultconfig.List)
	toml.Unmarshal(b, &defaultconfig)
	fmt.Println(defaultconfig.List)
	return nil
}

func main() {
	conffn := flag.String("config", "", "config file")
	flag.Parse()

	if *conffn != "" {
		err := ReadConfig(*conffn)
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

	recdir := filepath.Join(defaultconfig.Homedir, "rz2/recorder")
	records := make(map[string]*Record)
	nrecords := 0

	srvaddress, err := rz2.ServerAddress(defaultconfig.Server)
	if srvaddress == "" {
		log.Fatal(err)
	}
	client, err := StartSubscriber(srvaddress, defaultconfig.List, func(client mqtt.Client, msg mqtt.Message) {
		lis := strings.Split(msg.Topic(), "/")
		var rec *Record
		rec = records[lis[0]]
		if rec == nil {
			tmprecdir := filepath.Join(recdir, strings.Replace(lis[0], ":", "_", -1))
			os.MkdirAll(tmprecdir, 0755)
			r := NewRecord(tmprecdir, lis[0])
			err := r.setdest()
			if err != nil {
				log.Fatal(err)
			}
			records[lis[0]] = r
			nrecords++
		}
		err := records[lis[0]].record(msg)
		if err != nil {
			log.Println(err)
		}
	})

	ticker := time.NewTicker(time.Minute * 60)
	conticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			for _, r := range records {
				r.setdest()
			}
			if defaultconfig.Removehour > 0 {
				err := removeoldfiles(-1 * time.Duration(defaultconfig.Removehour) * time.Hour)
				if err != nil {
					log.Println(err)
				} else {
					log.Printf("removed files %d hours ago", defaultconfig.Removehour)
				}
			}
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
