package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	dproxy "github.com/koron/go-dproxy"
	"github.com/yofu/rz2"
)

var (
	nrow      = 50
	sortunits = true
	sortfunc  func(i, j int) bool

	units = make([]*Unit, 0)

	list *widgets.List
)

var (
	cafile  = ""
	crtfile = ""
	keyfile = ""
	homedir = os.Getenv("HOME")
)

type Unit struct {
	hardwareaddress string
	hostaddress     string
	hostname        string
	unittime        string
	location        string
	receivedtime    time.Time
	tag             []string
	filesize        int
}

func statusline(str string) {
	list.Rows[1] = fmt.Sprintf("STATUS: %s", str)
	ui.Render(list)
}

func recordfilesize(macaddress string) (int, error) {
	dir := filepath.Join(homedir, "rz2/recorder", strings.Replace(macaddress, ":", "_", -1))
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, err
	}
	rd, err := ioutil.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	sort.Slice(rd, func(i, j int) bool {
		return rd[j].ModTime().Before(rd[i].ModTime())
	})
	return int(rd[0].Size()), nil
}

func StartSubscriber(server string, topics []string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(server)
	opts.SetAutoReconnect(false)
	opts.SetClientID(fmt.Sprintf("rz2mon_%d", time.Now().UnixNano()))
	if strings.HasPrefix(server, "ssl") {
		tlsconfig, err := rz2.NewTLSConfig(cafile, crtfile, keyfile)
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tlsconfig)
	}

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		lis := strings.Split(msg.Topic(), "/")
		if len(lis) < 3 {
			return
		}
		var unit *Unit
		ct := time.Now()
		for _, u := range units {
			if u.hardwareaddress == lis[0] {
				unit = u
				unit.receivedtime = ct
				break
			}
		}
		if unit == nil {
			unit = &Unit{
				hardwareaddress: lis[0],
				receivedtime:    ct,
				tag:             make([]string, 0),
			}
			units = append(units, unit)
		}
		switch lis[2] {
		case "info":
			var v interface{}
			json.Unmarshal(msg.Payload(), &v)
			p := dproxy.New(v)
			var d dproxy.Drain
			unit.hostname = d.String(p.M("name"))
			unit.unittime = d.String(p.M("time"))
			unit.location = d.String(p.M("location"))
			unit.hostaddress = d.String(p.M("hostaddress"))
		default:
		}
		unit.tag = append(unit.tag, lis[2])
		filesize, err := recordfilesize(lis[0])
		if err != nil {
		} else {
			unit.filesize = filesize
		}
		if sortunits {
			sort.Slice(units, sortfunc)
		}
		for i := 0; i < nrow; i++ {
			if i >= len(units) {
				break
			}
			list.Rows[i+3] = fmt.Sprintf("%02d %-20s %12d %s %s %s %s %s %s", i+1, units[i].hostname, units[i].filesize, units[i].receivedtime.Format("2006-01-02T15:04:05"), units[i].tag[len(units[i].tag)-1], units[i].location, units[i].unittime, units[i].hardwareaddress, units[i].hostaddress)
		}
		ui.Render(list)
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		statusline(fmt.Sprintf("[not connected](fg:yellow): %s", time.Now().Format("15:04:05.000")))
		return client, token.Error()
	}
	statusline(fmt.Sprintf("[connected](fg:green): %s", time.Now().Format("15:04:05.000")))

	for _, t := range topics {
		client.Subscribe(t, 0, nil)
	}

	return client, nil
}

func CreateList(n int) {
	list = widgets.NewList()
	list.Title = "rz2mon"
	list.Rows = make([]string, n)
	list.TextStyle = ui.NewStyle(ui.ColorWhite)
	list.WrapText = false
	list.SetRect(0, 0, 150, n+5)
	ui.Render(list)
}

func main() {
	server := flag.String("server", "tcp://192.168.1.23:18884", "server url:port")
	cafn := flag.String("cafile", "", "ca file")
	crtfn := flag.String("crtfile", "", "crt file")
	keyfn := flag.String("keyfile", "", "key file")
	home := flag.String("home", "", "home directory")
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

	if err := ui.Init(); err != nil {
		log.Fatal(err)
	}
	defer ui.Close()

	CreateList(nrow)

	sortfunc = func(i, j int) bool {
		return units[i].receivedtime.After(units[j].receivedtime)
	}

	client, err := StartSubscriber(*server, []string{"#"})
	if err != nil && client == nil {
		log.Fatal(err)
	}
	list.Rows[0] = fmt.Sprintf("SERVER: %s", *server)

	list.Rows[2] = "NO NAME                 REC.SIZE     TIME                EXT   INFO"

	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-ticker.C:
				if client.IsConnected() {
					statusline(fmt.Sprintf("[connected](fg:green): %s", time.Now().Format("15:04:05.000")))
				} else {
					statusline(fmt.Sprintf("[not connected](fg:yellow): %s", time.Now().Format("15:04:05.000")))
					for {
						if client.IsConnected() {
							client.Subscribe("#", 0, nil)
							break
						}
						token := client.Connect()
						token.WaitTimeout(time.Second * 5)
					}
				}
			}
		}
	}()

	uiEvents := ui.PollEvents()
	for {
		e := <-uiEvents
		switch e.ID {
		case "q", "<C-c>":
			return
		case "s":
			sortunits = !sortunits
		case "t":
			sortfunc = func(i, j int) bool {
				return units[i].receivedtime.After(units[j].receivedtime)
			}
		case "n":
			sortfunc = func(i, j int) bool {
				return strings.Compare(units[i].hostname, units[j].hostname) < 0
			}
		case "i":
			sortfunc = func(i, j int) bool {
				return strings.Compare(units[i].hostaddress, units[j].hostaddress) < 0
			}
		}
		ui.Render(list)
	}
}
