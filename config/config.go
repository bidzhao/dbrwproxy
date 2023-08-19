package config

import (
	yaml "gopkg.in/yaml.v3"
	"io/ioutil"
)

type Config struct {
	PostgresProxies []Proxy `yaml:"PostgreSQL"`
	MysqlProxies    []Proxy `yaml:"MySQL"`
}

type Proxy struct {
	Name   string       `yaml:"Name"`
	Server ServerConfig `yaml:"ServerConfig"`
	Db     DB           `yaml:"DB"`
}

type ServerConfig struct {
	ProxyAddr string `yaml:"ProxyAddr"`
}

type DB struct {
	Main        MainDB        `yaml:"Main"`
	Secondaries []SecondaryDB `yaml:"Secondaries"`
}

type MainDB struct {
	Addr string `yaml:"Addr"`
}

type SecondaryDB struct {
	Name              string `yaml:"Name"`
	Host              string `yaml:"Host"`
	Port              int    `yaml:"Port"`
	User              string `yaml:"User"`
	Password          string `yaml:"Password"`
	DbName            string `yaml:"DbName"`
	Weight            int    `yaml:"Weight"`
	MaxIdleConnCount  int    `yaml:"MaxIdleConnCount"`
	MaxOpenConnsCount int    `yaml:"MaxOpenConnsCount"`
	ConnMaxLifetime   int    `yaml:"ConnMaxLifetime"`
}

func ReadConfig(name string) (Config, error) {
	content, err := ioutil.ReadFile(name)
	if err != nil {
		return Config{}, err
	}

	var conf Config
	err = yaml.Unmarshal(content, &conf)
	if err != nil {
		return Config{}, err
	}

	return conf, nil
}
