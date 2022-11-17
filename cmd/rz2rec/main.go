package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type Record struct {
	sync.Mutex
	path       string
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

func (r *Record) setdest() (string, string, error) {
	r.Lock()
	if r.dest != nil {
		r.dest.Close()
	}
	now := time.Now()
	name := strings.Replace(r.macaddress, ":", "_", -1)
	p := filepath.Join(r.dir, fmt.Sprintf("%s_%s.dat", name, now.Format("2006-01-02-15-04-05")))
	oldp := r.path
	r.path = p
	w, err := os.Create(p)
	if err != nil {
		return "", "", err
	}
	r.dest = w
	r.Unlock()
	return oldp, r.path, nil
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
			_, _, err := r.setdest()
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
	// ticker := time.NewTicker(time.Second * 10)
	conticker := time.NewTicker(time.Second)
	backupch := make(chan string)
	go func() {
		for {
			select {
			case p := <-backupch:
				oldpath := p
				macdir := filepath.Base(filepath.Dir(p))
				newpath := filepath.Join(defaultconfig.Backupdir, macdir, filepath.Base(p))
				os.MkdirAll(filepath.Dir(newpath), 0755)
				newfile, err := os.Create(newpath)
				if err != nil {
					fmt.Printf("backup: %s\n", err)
				}
				defer newfile.Close()
				oldfile, err := os.Open(oldpath)
				if err != nil {
					fmt.Printf("backup: %s\n", err)
				}
				defer oldfile.Close()
				_, err = io.Copy(newfile, oldfile)
				if err != nil {
					fmt.Println(err)
				}
				fmt.Printf("backup: %s -> %s\n", oldpath, newpath)
			}
		}
	}()
	for {
		select {
		case <-ticker.C:
			for _, r := range records {
				oldp, _, err := r.setdest()
				fmt.Println(oldp, err)
				if err != nil {
					log.Printf("setdest: %s\n", err)
				}
				backupch <- oldp
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
