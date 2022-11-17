package rz2

import (
	"fmt"
	"io/ioutil"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server string `toml:"server"`
	Cafile string `toml:"cafile"`
	Crtfile string `toml:"crtfile"`
	Keyfile string `toml:"keyfile"`
	Homedir string `toml:"homedir"`
	Backupdir string `toml:"backupdir"`
	Removehour int `toml:"removehour"`
	List []string `toml:"list"`
}

func (c *Config) Println() {
	fmt.Printf("server: %s\n", c.Server)
	fmt.Printf("cafile: %s\n", c.Cafile)
	fmt.Printf("crtfile: %s\n", c.Crtfile)
	fmt.Printf("keyfile: %s\n", c.Keyfile)
	fmt.Printf("homedir: %s\n", c.Homedir)
	fmt.Printf("backupdir: %s\n", c.Backupdir)
	fmt.Printf("removehour: %d\n", c.Removehour)
	fmt.Print("list:\n")
	for i, t := range c.List {
		fmt.Printf("    %d: %s\n", i, t)
	}
	fmt.Println("")
}

func (c *Config) ReadConfig(fn string) error {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}
	toml.Unmarshal(b, &c)
	fmt.Println(c.List)
	return nil
}
