package main

import (
	validator "./validator"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"text/template"
	"time"
)

type SMPPGateConfig struct {
	LogFile       string   `json:"logFile"`
	ConnectURI    []string `json:"connectURI"`
	MYSQLURI      string   `json:"mysql"`
	Listen        string   `json:"listen"`
	ForwardSecret string   `json:"forwardSecret"`
	ProjectPath   string   `json:"projectPath"`
	SendDisabled  bool     `json:"sendDisabled"`
}

type HttpError struct {
	Err string `json:"err"`
}

type HttpQueueSendReq struct {
	Phone string `json:"phone" validate:"regexp=^\\+7[0-9]{10}$"`
	From  string `json:"from" validate:"regexp=^[a-zA-Z0-9_]*$"`
	Text  string `json:"text" validate:"regexp=^.+$"`
}

type HttpQueueSendRes struct {
	HttpError
}

type HttpUnsentMessagesRes struct {
	HttpError
	Messages []Message `json:"messages"`
}

type DayReportData struct {
	Date         string
	DeliveredMsg []Message
	SentMsg      []Message
	QueuedMsg    []Message
	ErroredMsg   []Message
}

func ParseRequest(c *gin.Context, req interface{}) (err error) {
	if err = c.BindJSON(req); err != nil {
		panic(err)
	}
	if err = validator.Validate(req); err != nil {
		panic(err)
	}
	return
}

func panicHandler(c *gin.Context) {
	if e := recover(); e != nil {
		var err error = e.(error)
		log.Printf("[FATAL ERROR]: %s\n%s", err.Error(), debug.Stack())
		if c != nil {
			c.JSON(http.StatusInternalServerError, &HttpError{Err: err.Error()})
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags)

	if (len(os.Args)) < 2 {
		fmt.Println("USAGE: " + os.Args[0] + " <config file>")
		log.Fatalf("No config file")
	}

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err.Error())
	}
	if err := os.Chdir(dir); err != nil {
		panic(err.Error())
	}
	raw, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic("Error: Can't open config file")
	}
	var config SMPPGateConfig
	if err = json.Unmarshal(raw, &config); err != nil {
		panic("Error: read config: " + err.Error())
	}

	if config.LogFile != "" {
		logFile, err := os.OpenFile(config.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			panic("Error: Can't open log file " + config.LogFile + ": " + err.Error())
		}
		defer logFile.Close()
		log.SetOutput(logFile)
		gin.DefaultWriter = logFile
	}

	db, err := NewDBORM(config.MYSQLURI)
	if err != nil {
		panic(err)
	}

	smppworker, err := NewSMPPWorker(config.ConnectURI, db, config.SendDisabled)
	if err != nil {
		panic(err)
	}
	smppworker.Start()

	pRouter := gin.Default()
	pRouter.Use(func(c *gin.Context) {
		if config.ForwardSecret != c.Request.Header.Get("X-Forward-Secret") {
			c.String(http.StatusForbidden, "403 Forbidden")
			c.Abort()
		}
		c.Next()
	})
	pRouter.POST(config.ProjectPath+"/queueSend", func(c *gin.Context) {
		defer panicHandler(c)

		var req HttpQueueSendReq
		var res HttpQueueSendRes
		if err := ParseRequest(c, &req); err == nil {
			if err := db.Conn.Create(&Message{
				From:  req.From,
				Phone: req.Phone,
				Text:  req.Text,
			}).Error; err != nil {
				panic(err)
			} else {
				c.JSON(http.StatusOK, &res)
				smppworker.Flush()
			}
		}
	})
	pRouter.GET(config.ProjectPath+"/unsentMessages", func(c *gin.Context) {
		defer panicHandler(c)

		var res HttpUnsentMessagesRes
		if err := db.Conn.Where("status='errored' and try_count >= ?", SendMaxTryCount).Find(&res.Messages).Error; err == nil {
			c.JSON(http.StatusOK, &res)
		} else {
			panic(err)
		}
	})
	pRouter.GET(config.ProjectPath+"/dayReport", func(c *gin.Context) {
		defer panicHandler(c)

		var date time.Time
		if c.Request.URL.Query().Get("date") != "" {
			dateString := c.Request.URL.Query().Get("date") + "T00:00:00Z"
			var err error
			date, err = time.Parse(time.RFC3339, dateString)
			if err != nil {
				panic(err)
			}
		} else {
			date, err = time.Parse(time.RFC3339, time.Now().Format(time.RFC3339)[:10]+"T00:00:00Z")
		}

		dateTomorrow := date.Add(time.Hour * 24)

		var dayReportData DayReportData
		dayReportData.Date = date.Format(time.RFC3339)[:10]

		if err := db.Conn.Where("created_at >= ? and created_at < ? and status='delivered'",
			date, dateTomorrow).Find(&dayReportData.DeliveredMsg).Error; err == nil {
		} else {
			panic(err)
		}
		if err := db.Conn.Where("created_at >= ? and created_at < ? and status='sent'",
			date, dateTomorrow).Find(&dayReportData.SentMsg).Error; err == nil {
		} else {
			panic(err)
		}
		if err := db.Conn.Where("created_at >= ? and created_at < ? and (status='new' or (status='errored' and try_count < ?))",
			date, dateTomorrow, SendMaxTryCount).Find(&dayReportData.QueuedMsg).Error; err == nil {
		} else {
			panic(err)
		}
		if err := db.Conn.Where("created_at >= ? and created_at < ? and status='errored' and try_count >= ?",
			date, dateTomorrow, SendMaxTryCount).Find(&dayReportData.ErroredMsg).Error; err == nil {
		} else {
			panic(err)
		}
		t, err := template.ParseFiles("./dayReport.tmpl")
		if err != nil {
			panic(err)
		}
		buf := new(bytes.Buffer)
		if err := t.Execute(buf, dayReportData); err != nil {
			panic(err)
		}
		c.Data(http.StatusOK, "text/plain;charset=utf-8", buf.Bytes())
	})

	pRouter.Run(config.Listen)
}
