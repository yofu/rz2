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
	toml "github.com/pelletier/go-toml/v2"
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
	dir := filepath.Join(defaultconfig.Homedir, "rz2/recorder", strings.Replace(macaddress, ":", "_", -1))
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
		tlsconfig, err := rz2.NewTLSConfig(defaultconfig.Cafile, defaultconfig.Crtfile, defaultconfig.Keyfile)
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
	server := flag.String("server", "", "server url:port")
	conffn := flag.String("config", "", "config file")
	flag.Parse()

	if *conffn != "" {
		err := ReadConfig(*conffn)
		if err != nil {
			log.Fatal(err)
		}
	}
	if *server != "" {
		defaultconfig.Server = *server
	}
	if len(defaultconfig.List) == 0 {
		defaultconfig.List = []string{"#"}
	}

	defaultconfig.Println()

	if err := ui.Init(); err != nil {
		log.Fatal(err)
	}
	defer ui.Close()

	CreateList(nrow)

	sortfunc = func(i, j int) bool {
		return units[i].receivedtime.After(units[j].receivedtime)
	}

	srvaddress, err := rz2.ServerAddress(defaultconfig.Server)
	if srvaddress == "" {
		log.Fatal(err)
	}
	client, err := StartSubscriber(srvaddress, defaultconfig.List)
	if err != nil && client == nil {
		log.Fatal(err)
	}
	list.Rows[0] = fmt.Sprintf("SERVER: %s", srvaddress)

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
		case "m":
			sortfunc = func(i, j int) bool {
				return strings.Compare(units[i].hardwareaddress, units[j].hardwareaddress) < 0
			}
		}
		ui.Render(list)
	}
}
