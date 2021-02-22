package rz2

import (
	"fmt"
	"os"
)

func ServerAddress(cand string) (string, error) {
	if cand != "" {
		return cand, nil
	}
	if env := os.Getenv("MQTTSERVER"); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("mqtt server not found")
}
