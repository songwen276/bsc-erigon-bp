package pairconfig

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	MysqlUser string `toml:"mysql-user"`

	MysqlPwd string `toml:"mysql-pwd"`

	MysqlHost string `toml:"mysql-host"`

	MysqlDb string `toml:"mysql-db"`

	MysqlMaxOpenConns int `toml:"mysql-max-open-conns"`

	MysqlMaxIdleConns int `toml:"mysql-max-idle-conns"`

	MysqlConnMaxLifetime int `toml:"mysql-conn-max-lifetime"`

	PaircacheAbistr string `toml:"paircache-abistr"`

	PaircacheFrom string `toml:"paircache-from"`

	ClusterId int64 `toml:"cluster-id"`

	ClusterTotal int64 `toml:"cluster-total"`

	ConfigItemUrl string `toml:"config-item-url"`

	ChainId int64 `toml:"chain-id"`

	Type string `toml:"type"`

	EsGasLimit uint64 `toml:"esgaslimit"`

	OpenCache bool `toml:"open-cache"`
}

var DefaultConfig = Config{
	MysqlMaxOpenConns:    10,
	MysqlMaxIdleConns:    5,
	MysqlConnMaxLifetime: 3600,
	ClusterId:            1,
	ClusterTotal:         1,
}

type LocalConfig struct {
	TriangleFilterNum int `yaml:"triangleFilterNum"`
}

func LoadConfig(file string) (*LocalConfig, error) {
	// 打开文件
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 创建 Config 实例
	var cfg LocalConfig

	// 使用 YAML 解码
	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
