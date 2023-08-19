package main

import (
	"dbrwproxy/config"
	"dbrwproxy/proxy"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	configFile := flag.String("c", "config.yml", "Config file")
	flag.Parse()
	conf, err := config.ReadConfig(*configFile)
	if err != nil {
		log.Println("Failed to load config file", *configFile, err)
		os.Exit(1)
	}

	for _, proxyConf := range conf.PostgresProxies {
		go proxy.StartPostgres(proxyConf)
	}
	for _, proxyConf := range conf.MysqlProxies {
		go proxy.StartMysql(proxyConf)
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGINT)
	go func() {
		<-c
		for _, db := range proxy.PostgresDBs {
			db.Db.Close()
		}
		for _, db := range proxy.MysqlDBs {
			db.Db.Close()
		}
		os.Exit(1)
	}()

	if len(conf.PostgresProxies) > 0 || len(conf.MysqlProxies) > 0 {
		var wg sync.WaitGroup
		wg.Add(1)
		wg.Wait()
	} else {
		log.Println("No proxy instances found, please configure it in configuration file", *configFile)
	}
}
