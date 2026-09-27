package dns

import (
	"github.com/mizuchilabs/relayd/internal/dns/pihole"
)

func newPihole(env env) (client, error) {
	insecure, err := parseBool(env("INSECURE"), false)
	if err != nil {
		return nil, err
	}
	return pihole.New(env("URL"), env("TOKEN"), insecure), nil
}
