package mysqldb

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/jmoiron/sqlx"
	"sync"
	"time"
)

// DB 是全局数据库连接池
var (
	mysqldb         *sqlx.DB
	once            sync.Once
	User            string
	Password        string
	Hostname        string
	Dbname          string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
)

// InitDB 初始化数据库连接
func InitDB() {
	fmt.Println("init db", "User", User, "Password", Password, "Hostname", Hostname, "Dbname", Dbname, "MaxOpenConns", MaxOpenConns, "MaxIdleConns", MaxIdleConns)
	once.Do(func() {
		// 构建 DSN (Data Source Name)
		dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", User, Password, Hostname, Dbname)

		// 打开数据库连接
		var err error
		mysqldb, err = sqlx.Open("mysql", dsn)
		if err != nil {
			log.Error("Error opening database", err)
		}

		// 配置连接池
		mysqldb.SetMaxOpenConns(MaxOpenConns)
		mysqldb.SetMaxIdleConns(MaxIdleConns)
		mysqldb.SetConnMaxLifetime(ConnMaxLifetime)

		// 验证连接
		if err = mysqldb.Ping(); err != nil {
			log.Error("Error pinging database", err)
		}
	})
}

// GetDB 返回数据库连接池对象
func GetMysqlDB() *sqlx.DB {
	if mysqldb == nil {
		log.Error("Database not initialized. Call InitDB first.")
	}
	return mysqldb
}
