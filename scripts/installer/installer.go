package main

import (
	//"flag"
	"os"
	"bufio"
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"
	"github.com/pkg/errors"
)

var osVersion string
var crioVersion string

type InstallationMethod int
const (
	Apt InstallationMethod = itoa
	Yum
	Fedora
	Unsupported
)

var supportedApt := []string{
	"Debian_Unstable", "Debian_Testing", "xUbuntu_20.04",
}
var supportedYum := []string{
	"CentOS_8", "CentOS_8_Stream",
}
var supportedFedora := "Fedora"

func main() {
	// Parse CLI flags
	flag.StringVar(&osVersion, "os-version", "", "your operating system version")
	flag.StringVar(&crioVersion, "crio-version", "", "the desired crio version")
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatalf("unable to %v", err)
	}
}

func run() error {
	
	return installForVersion()
}

func installForVersion() InstallationMethod {

	if osVersion == supportedFedora {
		return Fedora
	}
	if isInSlice(osVersion, supportedApt) {
		return Apt
	}
	if isInSlice(osVersion, supportedYum) {
		return Yum
	}
	return Unsupported
}

func isInSlice(s string, slice []string) bool {
	for _, i := range slice {
		if s == i {
			return true
		}
	}
	return false
}

func installFedora() {
}
