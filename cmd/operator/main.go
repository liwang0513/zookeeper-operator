package main

import (
	"fmt"
	"flag"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/liwang-pivotal/zookeeper-operator/pkg/kube"
	"github.com/liwang-pivotal/zookeeper-operator/pkg/controller"
	"os/signal"
	"syscall"
)

var (
	appVersion = "0.0.1"

	printVersion bool
	baseImage    string
	kubeConfigFile  string
	masterHost   string

	logger = log.WithFields(log.Fields{
		"package": "main",
	})
)

func init() {
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")
	flag.StringVar(&baseImage, "baseImage", "liwang0513/docker-zookeeper-kubernetes:1.0.0_0", "Base image to use when spinning up the zookeeper components.")
	flag.StringVar(&kubeCfgFile, "kubecfg-file", "", "Location of kubecfg file for access to kubernetes master service; --kube_master_url overrides the URL part of this; if neither this nor --kube_master_url are provided, defaults to service account tokens")
	flag.StringVar(&masterHost, "masterhost", "http://127.0.0.1:8001", "Full url to k8s api server")
	flag.Parse()
}


func Main() int {
	if printVersion {
		fmt.Println("zookeeper-operator", appVersion)
		os.Exit(0)
	}

	log.Info("zookeeper operator starting up!")

	// Print params configured
	log.Info("Using Variables:")
	log.Infof("   baseImage: %s", baseImage)

	//Creating osSignals first so we can exit at any time.
	osSignals := make(chan os.Signal, 2)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGKILL, os.Interrupt)

	go func() {
		for {
			select {
			case sig := <-osSignals:
				logger.WithFields(log.Fields{"signal": sig}).Info("Got Signal from OS shutting Down: ")
				os.Exit(1)
			}
		}
	}()

	// Init
	kube, err := kube.New(kubeConfigFile, masterHost)

	controller, err := controller.New("zookeeper-cluster", k8sclient)
	if err != nil {
		log.Error("Could not init Controller! ", err)
		return 1
	}

	// Kick it off
	controller.Run()

	return 0
}

func main() {
	os.Exit(Main())
}