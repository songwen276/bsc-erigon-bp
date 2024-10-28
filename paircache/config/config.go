package pairconfig

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
}

var DefaultConfig = Config{
	MysqlMaxOpenConns:    10,
	MysqlMaxIdleConns:    5,
	MysqlConnMaxLifetime: 3600,
	ClusterId:            1,
	ClusterTotal:         1,
}
