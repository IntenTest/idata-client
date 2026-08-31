package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/user"
	"strings"
)

type localDeviceIdentity struct {
	Username   string
	Hostname   string
	LocalIP    string
	MACAddress string
}

func readLocalDeviceIdentity() localDeviceIdentity {
	identity := localDeviceIdentity{}
	identity.Hostname, _ = os.Hostname()
	if current, err := user.Current(); err == nil {
		identity.Username = current.Username
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return identity
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.To4() == nil {
				continue
			}
			identity.LocalIP = ip.String()
			identity.MACAddress = strings.ToUpper(networkInterface.HardwareAddr.String())
			return identity
		}
	}
	return identity
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
