package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/yofu/rz2"
)

var (
	cafile  = ""
	crtfile = ""
	keyfile = ""
)

type Client struct {
	hostname   string
	ipaddress  string
	macaddress string
	column     int
	datatype   Datatype
}

type Datatype int

const (
	Strain Datatype = iota
	Acc
	Clock
	Ill
	Ir
	Gill
)

var (
	clients = make([]*Client, 0)
)

func ReadClient(fn string) error {
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		t := scanner.Text()
		if strings.HasPrefix(t, "#") {
			continue
		}
		lis := strings.Fields(t)
		if len(lis) < 3 {
			fmt.Println("ReadClient: not enough data")
			continue
		}
		col := 0
		ty := Strain
		if len(lis) > 3 {
			switch lis[3] {
			case "1":
				col = 1
			}
			if len(lis) > 4 {
				switch strings.ToLower(lis[4]) {
				case "strain":
					ty = Strain
				case "acc":
					ty = Acc
				case "clock":
					ty = Clock
				case "ill":
					ty = Ill
				case "ir":
					ty = Ir
				case "gill":
					ty = Gill
				}
			}
		}
		newclient := &Client{
			hostname:   lis[0],
			ipaddress:  lis[1],
			macaddress: lis[2],
			column:     col,
			datatype:   ty,
		}
		clients = append(clients, newclient)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func accStats(client *Client, msg mqtt.Message) (string, error) {
	var send_time int64
	buf := bytes.NewReader(msg.Payload()[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	acc, xind, err := rz2.ConvertAccPacket(msg.Payload()[12:])
	if err != nil {
		return "", err
	}
	acc = acc[xind : len(acc)-3+xind]
	xacc := make([]float64, len(acc)/3)
	yacc := make([]float64, len(acc)/3)
	zacc := make([]float64, len(acc)/3)
	avex := 0.0
	avey := 0.0
	avez := 0.0
	for i := 0; i < len(xacc); i++ {
		xacc[i] = acc[3*i]
		yacc[i] = acc[3*i+1]
		zacc[i] = acc[3*i+2]
		avex += xacc[i]
		avey += yacc[i]
		avez += zacc[i]
	}
	avex /= float64(len(xacc))
	avey /= float64(len(yacc))
	avez /= float64(len(zacc))
	return fmt.Sprintf("%s: %s, data= %d, ave(x/y/z)= %.3f/%.3f/%.3f", client.hostname, rz2.ConvertUnixtime(send_time).Format("15:04:05.000"), len(xacc), avex, avey, avez), nil
}

func strainStats(client *Client, msg mqtt.Message) (string, error) {
	mtime, strain, err := rz2.ConvertStrain(msg.Payload(), 103, false)
	if err != nil {
		return "", err
	}
	if len(strain) == 0 {
		return "", fmt.Errorf("no data")
	}
	min := strain[0]
	max := strain[0]
	for i := 0; i < len(strain); i++ {
		if strain[i] < min {
			min = strain[i]
		}
		if strain[i] > max {
			max = strain[i]
		}
	}
	color := "white"
	threshold := 1<<23 - 1000
	if int(min) > threshold {
		color = "red"
	}
	lis := strings.Split(msg.Topic(), "/")
	if len(lis) < 2 {
		return "", fmt.Errorf("unknown client: %s", msg.Topic())
	}
	return fmt.Sprintf("%s/%s: %s, data= %d, min/max= [%d/%d](fg:%s)", client.hostname, lis[1], rz2.ConvertUnixtime(mtime[len(mtime)-1]).Format("15:04:05.000"), len(strain), int32(min), int32(max), color), nil
}

func clockStats(client *Client, msg mqtt.Message) (string, error) {
	var send_time int64
	var data int32
	buf := bytes.NewReader(msg.Payload()[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	buf = bytes.NewReader(msg.Payload()[8:12])
	binary.Read(buf, binary.BigEndian, &data)
	// fmt.Println(data, msg.Payload()[8:12])
	lis := strings.Split(msg.Topic(), "/")
	if len(lis) < 2 {
		return "", fmt.Errorf("unknown client: %s", msg.Topic())
	}
	return fmt.Sprintf("%s/%s: %s, data= %d", client.hostname, lis[1], rz2.ConvertUnixtime(send_time).Format("15:04:05.000"), data), nil
}

func illStats(client *Client, msg mqtt.Message) (string, error) {
	send_time, data, err := rz2.ConvertIll(msg.Payload())
	if err != nil {
		return "", err
	}
	ave := 0.0
	for i :=0; i < len(data); i++ {
		ave += float64(data[i])
	}
	ave /= float64(len(data))
	return fmt.Sprintf("%s: %s, data= %d, ave= %.3f", client.hostname, rz2.ConvertUnixtime(send_time).Format("15:04:05.000"), len(data), ave), nil
}

func irStats(client *Client, msg mqtt.Message) (string, error) {
	send_time, data, err := rz2.ConvertIR(msg.Payload())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s, data= %d", client.hostname, rz2.ConvertUnixtime(send_time).Format("15:04:05.000"), data), nil
}

func gillStats(client *Client, msg mqtt.Message) (string, error) {
	send_time, data, err := rz2.ConvertGill(msg.Payload())
	if err != nil {
		return "", err
	}
	lis := strings.Split(data, ",")
	return fmt.Sprintf("%s: %s, spped= %s, angle= %s", client.hostname, rz2.ConvertUnixtime(send_time).Format("15:04:05.000"), lis[2], lis[1]), nil
}

func getLatestFileSize(path string) (int64, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !stat.IsDir() {
		return 0, fmt.Errorf("not directory")
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	lis, err := f.Readdir(-1)
	if err != nil {
		return 0, err
	}
	sort.Slice(lis, func(i, j int) bool {
		return lis[i].ModTime().After(lis[j].ModTime())
	})
	return lis[0].Size(), nil
}

func main() {
	server := flag.String("server", "", "server url:port")
	cfn := flag.String("client", "address.txt", "client data")
	cafn := flag.String("cafile", "", "ca file")
	crtfn := flag.String("crtfile", "", "crt file")
	keyfn := flag.String("keyfile", "", "key file")
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

	err := ReadClient(*cfn)
	if err != nil {
		log.Fatal(err)
	}
	if len(clients) == 0 {
		log.Fatal("no client")
	}

	leftkeys := make([]string, 0)
	rightkeys := make([]string, 0)
	clientkeys := make(map[string]*Client)
	for _, c := range clients {
		switch c.datatype {
		case Strain:
			if c.column == 0 {
				for j := 1; j <= 4; j++ {
					key := fmt.Sprintf("%s/%02d/str01", c.macaddress, j)
					leftkeys = append(leftkeys, key)
					clientkeys[key] = c
				}
			} else {
				for j := 1; j <= 4; j++ {
					key := fmt.Sprintf("%s/%02d/str01", c.macaddress, j)
					rightkeys = append(rightkeys, key)
					clientkeys[key] = c
				}
			}
		case Acc:
			if c.column == 0 {
				key := fmt.Sprintf("%s/01/acc02", c.macaddress)
				leftkeys = append(leftkeys, key)
				clientkeys[key] = c
			} else {
				key := fmt.Sprintf("%s/01/acc02", c.macaddress)
				rightkeys = append(rightkeys, key)
				clientkeys[key] = c
			}
		case Clock:
			if c.column == 0 {
				key := fmt.Sprintf("%s/01/clk01", c.macaddress)
				leftkeys = append(leftkeys, key)
				clientkeys[key] = c
			} else {
				key := fmt.Sprintf("%s/01/clk01", c.macaddress)
				rightkeys = append(rightkeys, key)
				clientkeys[key] = c
			}
		case Ill:
			if c.column == 0 {
				key := fmt.Sprintf("%s/01/ill01", c.macaddress)
				leftkeys = append(leftkeys, key)
				clientkeys[key] = c
			} else {
				key := fmt.Sprintf("%s/01/ill01", c.macaddress)
				rightkeys = append(rightkeys, key)
				clientkeys[key] = c
			}
		case Ir:
			if c.column == 0 {
				key := fmt.Sprintf("%s/01/ir01", c.macaddress)
				leftkeys = append(leftkeys, key)
				clientkeys[key] = c
			} else {
				key := fmt.Sprintf("%s/01/ir01", c.macaddress)
				rightkeys = append(rightkeys, key)
				clientkeys[key] = c
			}
		case Gill:
			if c.column == 0 {
				key := fmt.Sprintf("%s/01/gill01", c.macaddress)
				leftkeys = append(leftkeys, key)
				clientkeys[key] = c
			} else {
				key := fmt.Sprintf("%s/01/gill01", c.macaddress)
				rightkeys = append(rightkeys, key)
				clientkeys[key] = c
			}
		}
	}

	if err := ui.Init(); err != nil {
		log.Fatal(err)
	}
	defer ui.Close()

	ll := widgets.NewList()
	ll.Title = "Value Monitor"
	ll.Rows = make([]string, len(leftkeys))
	for i, k := range leftkeys {
		ll.Rows[i] = k
	}
	ll.TextStyle = ui.NewStyle(ui.ColorWhite)
	ll.WrapText = false
	ll.SetRect(0, 0, 80, len(leftkeys)+2)
	ui.Render(ll)

	rl := widgets.NewList()
	rl.Title = "Value Monitor"
	rl.Rows = make([]string, len(rightkeys))
	for i, k := range rightkeys {
		rl.Rows[i] = k
	}
	rl.TextStyle = ui.NewStyle(ui.ColorWhite)
	rl.WrapText = false
	rl.SetRect(80, 0, 160, len(rightkeys)+2)
	ui.Render(rl)

	srvaddress, err := rz2.ServerAddress(*server)
	if srvaddress == "" {
		log.Fatal(err)
	}
	sl := widgets.NewList()
	sl.Title = "Status"
	sl.Rows = make([]string, 4)
	sl.Rows[0] = fmt.Sprintf("SERVER: %s", srvaddress)
	sl.Rows[1] = "STATUS: [connected](fg:green)"
	sl.Rows[2] = "[no error](fg:green)"
	sl.Rows[3] = "file size"
	sl.TextStyle = ui.NewStyle(ui.ColorWhite)
	sl.WrapText = false
	sl.SetRect(80, len(rightkeys)+3, 160, len(rightkeys)+9)
	ui.Render(sl)

	id := fmt.Sprintf("valmon_go_%d", time.Now().UnixNano())

	var srcclient mqtt.Client

	srcopts := mqtt.NewClientOptions()
	srcopts.AddBroker(srvaddress)
	srcopts.SetAutoReconnect(false)
	srcopts.SetClientID(id)
	if strings.HasPrefix(srvaddress, "ssl") {
		tlsconfig, err := rz2.NewTLSConfig(cafile, crtfile, keyfile)
		if err != nil {
			log.Fatal(err)
		}
		srcopts.SetTLSConfig(tlsconfig)
	}
	srcopts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		lis := strings.Split(msg.Topic(), "/")
		cl := clientkeys[msg.Topic()]
		if cl == nil {
			return
		}
		switch lis[2] {
		case "acc02":
			status, err := accStats(cl, msg)
			if err != nil {
				sl.Rows[2] = fmt.Sprintf("[%s:%s](fg:red)", cl.hostname, err)
				ui.Render(sl)
				return
			}
			switch cl.column {
			case 0:
				for i, k := range leftkeys {
					if k == msg.Topic() {
						ll.Rows[i] = status
						ui.Render(ll)
						break
					}
				}
			case 1:
				for i, k := range rightkeys {
					if k == msg.Topic() {
						rl.Rows[i] = status
						ui.Render(rl)
						break
					}
				}
			}
		case "str01":
			status, err := strainStats(cl, msg)
			if err != nil {
				sl.Rows[2] = fmt.Sprintf("[%s/%d:%s](fg:red)", cl.hostname, lis[1], err)
				ui.Render(sl)
				return
			}
			switch cl.column {
			case 0:
				for i, k := range leftkeys {
					if k == msg.Topic() {
						ll.Rows[i] = status
						ui.Render(ll)
						break
					}
				}
			case 1:
				for i, k := range rightkeys {
					if k == msg.Topic() {
						rl.Rows[i] = status
						ui.Render(rl)
						break
					}
				}
			}
		case "clk01":
			status, err := clockStats(cl, msg)
			if err != nil {
				sl.Rows[2] = fmt.Sprintf("[%s/%d:%s](fg:red)", cl.hostname, lis[1], err)
				ui.Render(sl)
				return
			}
			switch cl.column {
			case 0:
				for i, k := range leftkeys {
					if k == msg.Topic() {
						ll.Rows[i] = status
						ui.Render(ll)
						break
					}
				}
			case 1:
				for i, k := range rightkeys {
					if k == msg.Topic() {
						rl.Rows[i] = status
						ui.Render(rl)
						break
					}
				}
			}
		case "ill01":
			status, err := illStats(cl, msg)
			if err != nil {
				sl.Rows[2] = fmt.Sprintf("[%s:%s](fg:red)", cl.hostname, err)
				ui.Render(sl)
				return
			}
			switch cl.column {
			case 0:
				for i, k := range leftkeys {
					if k == msg.Topic() {
						ll.Rows[i] = status
						ui.Render(ll)
						break
					}
				}
			case 1:
				for i, k := range rightkeys {
					if k == msg.Topic() {
						rl.Rows[i] = status
						ui.Render(rl)
						break
					}
				}
			}
		case "ir01":
			status, err := irStats(cl, msg)
			if err != nil {
				sl.Rows[2] = fmt.Sprintf("[%s:%s](fg:red)", cl.hostname, err)
				ui.Render(sl)
				return
			}
			switch cl.column {
			case 0:
				for i, k := range leftkeys {
					if k == msg.Topic() {
						ll.Rows[i] = status
						ui.Render(ll)
						break
					}
				}
			case 1:
				for i, k := range rightkeys {
					if k == msg.Topic() {
						rl.Rows[i] = status
						ui.Render(rl)
						break
					}
				}
			}
		case "gill01":
			status, err := gillStats(cl, msg)
			if err != nil {
				sl.Rows[2] = fmt.Sprintf("[%s:%s](fg:red)", cl.hostname, err)
				ui.Render(sl)
				return
			}
			switch cl.column {
			case 0:
				for i, k := range leftkeys {
					if k == msg.Topic() {
						ll.Rows[i] = status
						ui.Render(ll)
						break
					}
				}
			case 1:
				for i, k := range rightkeys {
					if k == msg.Topic() {
						rl.Rows[i] = status
						ui.Render(rl)
						break
					}
				}
			}
		default:
		}
	})

	srcclient = mqtt.NewClient(srcopts)
	token := srcclient.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		sl.Rows[1] = fmt.Sprintf("STATUS: [not connected](fg:yellow): %s", time.Now().Format("15:04:05.000"))
		sl.Rows[2] = fmt.Sprintf("[%s](fg:red)", token.Error())
		ui.Render(sl)
	} else {
		srcclient.Subscribe("#", 0, nil)
	}

	ticker := time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				if srcclient.IsConnected() {
					sl.Rows[1] = fmt.Sprintf("STATUS: [connected](fg:green): %s", time.Now().Format("15:04:05.000"))
					sl.Rows[2] = "[no error](fg:green)"
				} else {
					sl.Rows[1] = fmt.Sprintf("STATUS: [not connected](fg:yellow): %s", time.Now().Format("15:04:05.000"))
					ui.Render(sl)
					for {
						if srcclient.IsConnected() {
							srcclient.Subscribe("#", 0, nil)
							break
						}
						token := srcclient.Connect()
						token.WaitTimeout(time.Second * 5)
						if token.Error() != nil {
							sl.Rows[2] = fmt.Sprintf("[%s](fg:red)", token.Error())
							ui.Render(sl)
						}
					}
				}
				ui.Render(sl)
			}
		}
	}()

	uiEvents := ui.PollEvents()
	for {
		e := <-uiEvents
		switch e.ID {
		case "q", "<C-c>":
			return
		}
		ui.Render(ll)
		ui.Render(rl)
		ui.Render(sl)
	}
}
