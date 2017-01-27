package main

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"log"
	"time"
)

type Model struct {
	ID        uint64     `gorm:"primary_key" json:"-"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
	DeletedAt *time.Time `json:"-"`
}

type Message struct {
	Model
	From      string `sql:"size:32" json:"from"`
	Phone     string `sql:"size:12;index" json:"phone"`
	Text      string `sql:"type:text" json:"text"`
	Status    string `sql:"type:ENUM('new', 'sent', 'delivered', 'errored');DEFAULT:'new';index" json:"status"`
	TryCount  int32  `sql:"DEFAULT:0" json:"-"`
	MessageId string `sql:"size:32;index" json:"messageId"`
	LastError string `sql:"type:text" json:"lastError"`
}

type DBORM struct {
	Conn *gorm.DB
}

func NewDBORM(MYSQL_URI string) (*DBORM, error) {

	conn, err := gorm.Open("mysql", MYSQL_URI)
	if err != nil {
		return nil, err
	}
	conn.SingularTable(true)
	log.Println("Start migrate tables")
	conn.AutoMigrate(Message{})

	return &DBORM{Conn: conn}, nil
}
