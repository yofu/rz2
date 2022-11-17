package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/yofu/rz2"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

var (
	defaultconfig   = &rz2.Config{
		Server: "tcp://192.168.0.100:1883",
		Cafile: "",
		Crtfile: "",
		Keyfile: "",
		Homedir: os.Getenv("HOME"),
		Removehour: 0,
		List: make([]string, 0),
	}
)

func getPortInfo() (string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	for _, port := range ports {
		fmt.Println(port.Name, port.IsUSB, port.VID)
		if port.IsUSB && port.VID == "0403" {
			return port.Name, nil
		}
	}
	return "", fmt.Errorf("cannot find")
}

func StartPublisher(server string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(server)
	opts.SetAutoReconnect(false)
	opts.SetClientID(fmt.Sprintf("gill_%d", time.Now().UnixNano()))

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		return client, token.Error()
	}

	return client, nil
}

func getMacAddress() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	var currentIP string
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				currentIP = ipnet.IP.String()
				break
			}
		}
	}
	var currentNetworkHardwareName string
	interfaces, _ := net.Interfaces()
	for _, interf := range interfaces {
		if addrs, err := interf.Addrs(); err == nil {
			for _, addr := range addrs {
				if strings.Contains(addr.String(), currentIP) {
					currentNetworkHardwareName = interf.Name
					break
				}
			}
		}
	}
	netInterface, err := net.InterfaceByName(currentNetworkHardwareName)
	if err != nil {
		return "", err
	}
	return netInterface.HardwareAddr.String(), nil
}

func parseMessage(message string) ([]byte, error) {
	if len(message) < 22 {
		return nil, fmt.Errorf("not enough message length")
	}
	buf := new(bytes.Buffer)
	ct := time.Now().UnixMilli()
	err := binary.Write(buf, binary.BigEndian, ct)
	if err != nil {
		return nil, err
	}
	var size int32 = 16
	err = binary.Write(buf, binary.BigEndian, size)
	if err != nil {
		return nil, err
	}
	lis := strings.Split(message, ",")
	angle, err := strconv.ParseFloat(lis[1], 64)
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, angle)
	if err != nil {
		return nil, err
	}
	speed, err := strconv.ParseFloat(lis[2], 64)
	if err != nil {
		return nil, err
	}
	err = binary.Write(buf, binary.BigEndian, speed)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func main() {
	server := flag.String("server", "", "server url:port")
	conffn := flag.String("config", "", "config file")
	flag.Parse()

	if *conffn != "" {
		err := defaultconfig.ReadConfig(*conffn)
		if err != nil {
			log.Fatal(err)
		}
	}
	if *server != "" {
		defaultconfig.Server = *server
	}

	defaultconfig.Println()

	srvaddress, err := rz2.ServerAddress(defaultconfig.Server)
	if srvaddress == "" {
		log.Fatal(err)
	}
	client, err := StartPublisher(srvaddress)
	if err != nil && client == nil {
		log.Fatal(err)
	}

	macaddress, err := getMacAddress()
	if err != nil {
		log.Fatal(err)
	}
	mqtttopic := fmt.Sprintf("%s/01/gill01", macaddress)
	fmt.Println(mqtttopic)

	portName, err := getPortInfo()
	if err != nil {
		log.Fatal(err)
	}

	mode := &serial.Mode{
		BaudRate: 9600,
	}
	port, err := serial.Open(portName, mode)
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.NewTicker(time.Second)

	currentmessage := ""
	go func() {
		for {
			select {
			case <-ticker.C:
				b, err := parseMessage(currentmessage)
				if err != nil {
					fmt.Println(err)
				}
				if !client.IsConnected() {
					for {
						if client.IsConnected() {
							break
						}
						token := client.Connect()
						token.WaitTimeout(time.Second * 5)
					}
				}
				token := client.Publish(mqtttopic, 0, false, b)
				fmt.Println(b)
				token.Wait()
			}
		}
	}()

	scanner := bufio.NewScanner(port)
	for scanner.Scan() {
		currentmessage = scanner.Text()
	}
}
