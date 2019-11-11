package y

import (
	"errors"
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"time"
)

type DB struct {
	*gorm.DB
}

type DBConfig struct {
	Type string

	Username string
	Password string
	Protocol string
	Address  string
	Port     int
	Database string
	Charset  string

	EnableLog   bool
	MaxIdle     int
	MaxOpen     int
	MaxConnLife time.Duration
}

func NewDB(config *DBConfig) (db *DB, err error) {
	if config == nil {
		err = errors.New("nil parameter config")
		return
	}

	var connectStr string
	if config.Protocol == "unix" {
		connectStr = fmt.Sprintf("%s:%s@%s(%s)/%s?charset=%s&parseTime=True&loc=Local",
			config.Username, config.Password, config.Protocol, config.Address, config.Database, config.Charset)
	} else {
		connectStr = fmt.Sprintf("%s:%s@%s(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
			config.Username, config.Password, config.Protocol, config.Address, config.Port, config.Database, config.Charset)
	}

	d, err := gorm.Open(config.Type, connectStr)
	if err != nil {
		return
	}

	d.LogMode(config.EnableLog)
	d.DB().SetMaxIdleConns(config.MaxIdle)
	d.DB().SetMaxOpenConns(config.MaxOpen)
	d.DB().SetConnMaxLifetime(config.MaxConnLife)

	db = &DB{
		DB: d,
	}

	return
}

func (db *DB) InitTable(values ...interface{}) {
	err := db.AutoMigrate(values...).Error
	if err != nil {
		panic(err)
	}
}

func NewDBFromConfigFile(fileName string) (db *DB, err error) {
	var cfg DBConfig
	err = LoadXml(fileName, &cfg)
	if err != nil {
		return
	}

	return NewDB(&cfg)
}
