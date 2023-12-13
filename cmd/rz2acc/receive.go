package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/subcommands"
	"github.com/yofu/rz2"
)

type receiveCmd struct {
	server string
	macaddress string
}

func (*receiveCmd) Name() string{
	return "receive"
}

func (*receiveCmd) Synopsis() string{
	return "receive acceleration data"
}

func (*receiveCmd) Usage() string {
	return "receive [-server] [-macaddress]"
}

func (r *receiveCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&r.server, "server", "", "server url:port")
	f.StringVar(&r.macaddress, "macaddress", "", "mac address")
}

func (r *receiveCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	id := fmt.Sprintf("rz2acc_go_%d", time.Now().UnixNano())
	var srcclient mqtt.Client

	srvaddress, err := rz2.ServerAddress(r.server)
	if srvaddress == "" {
		log.Printf("[receive] %v\n", err)
		return subcommands.ExitFailure
	}
	srcopts := mqtt.NewClientOptions()
	srcopts.AddBroker(srvaddress)
	srcopts.SetAutoReconnect(false)
	srcopts.SetClientID(id)
	srcopts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		lis := strings.Split(msg.Topic(), "/")
		switch lis[2] {
		case "acc02":
			status, err := accStats(msg.Payload())
			if err != nil {
				log.Fatal("[receive] %v\n", err)
			}
			fmt.Printf("%s, %s\n", msg.Topic(), status)
			os.Exit(0)
		default:
		}
	})

	srcclient = mqtt.NewClient(srcopts)
	token := srcclient.Connect()
	token.WaitTimeout(time.Second * 5)
	if token.Error() != nil {
		log.Printf("[receive] %v\n", token.Error())
		return subcommands.ExitFailure
	} else {
		if r.macaddress == "" {
			srcclient.Subscribe("#", 0, nil)
		} else {
			srcclient.Subscribe(fmt.Sprintf("%s/+/+", r.macaddress), 0, nil)
		}
	}
	for {
	}
}

