package main

import (
	"errors"
	"fmt"
	"github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutext"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	StatusNotConnected = 0
	StatusConnected    = 1
)

var (
	ErrorOnlySMPPSchemeWasSupported       = errors.New("Only 'smpp' scheme was supported")
	ErrorNoTransceiversConnected          = errors.New("No transceivers connected")
	SendTryInterval                       = time.Minute * 30
	SendMaxTryCount                 int32 = 3
	SendInterval                          = time.Minute
	MessagesPerInterval                   = 30
)

type SMPPDeliverInfo struct {
	messageId string
	stat      string
	err       string
}

type SMPPTransceiver struct {
	Addr          string
	User          string
	Password      string
	SourceAddrTON uint8
	SourceAddrNPI uint8
	DestAddrTON   uint8
	DestAddrNPI   uint8
	Status        smpp.ConnStatusID
	trx           *smpp.Transceiver
	DeliveryCh    chan SMPPDeliverInfo
}

func NewSMPPTransceiver(uri string, DeliveryCh chan SMPPDeliverInfo) (*SMPPTransceiver, error) {
	connectURL, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	if connectURL.Scheme != "smpp" {
		return nil, ErrorOnlySMPPSchemeWasSupported
	}

	query := connectURL.Query()
	var SourceAddrTON int
	if len(query["SourceAddrTON"]) != 0 {
		SourceAddrTON, _ = strconv.Atoi(query["SourceAddrTON"][0])
	}
	var SourceAddrNPI int
	if len(query["SourceAddrTON"]) != 0 {
		SourceAddrNPI, _ = strconv.Atoi(query["SourceAddrNPI"][0])
	}
	var DestAddrTON int
	if len(query["DestAddrTON"]) != 0 {
		DestAddrTON, _ = strconv.Atoi(query["DestAddrTON"][0])
	}
	var DestAddrNPI int
	if len(query["DestAddrNPI"]) != 0 {
		DestAddrNPI, _ = strconv.Atoi(query["DestAddrNPI"][0])
	}

	password, _ := connectURL.User.Password()
	return &SMPPTransceiver{
		Addr:          connectURL.Host,
		User:          connectURL.User.Username(),
		Password:      password,
		SourceAddrTON: (uint8)(SourceAddrTON),
		SourceAddrNPI: (uint8)(SourceAddrNPI),
		DestAddrTON:   (uint8)(DestAddrTON),
		DestAddrNPI:   (uint8)(DestAddrNPI),
		DeliveryCh:    DeliveryCh,
	}, nil
}

func (t *SMPPTransceiver) Handler(p pdu.Body) {
	switch p.Header().ID {
	case pdu.DeliverSMID:
		fields := p.Fields()
		src := fields[pdufield.SourceAddr]
		dst := fields[pdufield.DestinationAddr]
		txt := fields[pdufield.ShortMessage]
		if txt != nil {
			props := strings.Split(txt.String(), " ")
			var deliveryInfo SMPPDeliverInfo
			for _, prop := range props {
				if prop != "" {
					keyvalue := strings.Split(prop, ":")
					if len(keyvalue) > 1 {
						if keyvalue[0] == "id" {
							deliveryInfo.messageId = keyvalue[1]
						}
						if keyvalue[0] == "stat" {
							deliveryInfo.stat = keyvalue[1]
						}
						if keyvalue[0] == "err" {
							deliveryInfo.err = keyvalue[1]
						}
					}
				}
			}
			if deliveryInfo.messageId != "" {
				t.DeliveryCh <- deliveryInfo
			}
		}
		log.Printf("Info: Short message from=%q to=%q: %q", src, dst, txt)
	}
}

func (t *SMPPTransceiver) Work() {
	t.trx = &smpp.Transceiver{
		Addr:    t.Addr,
		User:    t.User,
		Passwd:  t.Password,
		Handler: t.Handler,
	}
	conn := t.trx.Bind()
	go func() {
		for c := range conn {
			log.Println("Info: SMPP connection status:", c.Status())
			t.Status = c.Status()
		}
	}()
	log.Printf("Info: SMPP worker addr=%q started\n", t.Addr)
}

func (t *SMPPTransceiver) SendMessage(from string, dest string, text string) (string, error) {

	sm, err := t.trx.Submit(&smpp.ShortMessage{
		Src:           from,
		Dst:           dest,
		Text:          pdutext.UCS2(text),
		Register:      smpp.FinalDeliveryReceipt,
		SourceAddrTON: t.SourceAddrTON,
		SourceAddrNPI: t.SourceAddrNPI,
		DestAddrTON:   t.DestAddrTON,
		DestAddrNPI:   t.DestAddrNPI,
	})
	if err == nil {
		return sm.RespID(), nil
	} else {
		return "", err
	}
}

type SMPPWorker struct {
	transceiver  []*SMPPTransceiver
	db           *DBORM
	DeliveryCh   chan SMPPDeliverInfo
	FlushCh      chan bool
	sendDisabled bool
}

func NewSMPPWorker(connectURI []string, db *DBORM, sendDisabled bool) (*SMPPWorker, error) {
	Transceiver := make([]*SMPPTransceiver, 0)
	DeliveryCh := make(chan SMPPDeliverInfo)
	for _, uri := range connectURI {
		t, err := NewSMPPTransceiver(uri, DeliveryCh)
		if err != nil {
			return nil, err
		}
		Transceiver = append(Transceiver, t)
	}
	return &SMPPWorker{transceiver: Transceiver,
		db:           db,
		DeliveryCh:   DeliveryCh,
		FlushCh:      make(chan bool),
		sendDisabled: sendDisabled,
	}, nil
}

func (w *SMPPWorker) GetTransceiver() (*SMPPTransceiver, error) {
	for _, transceiver := range w.transceiver {
		if transceiver.Status == smpp.Connected {
			return transceiver, nil
		}
	}
	return nil, ErrorNoTransceiversConnected
}

func (w *SMPPWorker) Start() {
	for _, transceiver := range w.transceiver {
		transceiver.Work()
	}
	go func() {
		for deliveryInfo := range w.DeliveryCh {
			var message Message
			if err := w.db.Conn.Where("message_id = ? and status = 'sent'", deliveryInfo.messageId).First(&message).Error; err != nil {
				log.Printf("Error: Can't find sent message with id=%q\n", deliveryInfo.messageId)
			} else {
				if deliveryInfo.stat == "DELIVRD" {
					message.Status = "delivered"
				}
				if deliveryInfo.stat == "REJECTD" {
					message.Status = "errored"
					var errCode uint32
					fmt.Sscanf(deliveryInfo.err, "%x", &errCode)
					pduStatus := (pdu.Status)(errCode)
					message.LastError = pduStatus.Error()
					message.TryCount = SendMaxTryCount
				}
				w.db.Conn.Save(&message)
				log.Printf("Info: Message with id=%q changed status to '%s'", deliveryInfo.messageId, message.Status)
			}
		}
	}()
	go func() {
		for {
			<-w.FlushCh
			var messages []Message
			var oldUpdated time.Time = time.Now().Add(-SendTryInterval)
			err := w.db.Conn.Limit(MessagesPerInterval).Where("status = 'new' or status='errored' and updated_at < ? and try_count < ?",
				oldUpdated, SendMaxTryCount).Find(&messages).Error
			if err == nil {
				log.Printf("Info: Messages in queue: %d\n", len(messages))
				if w.sendDisabled {
					log.Printf("Warning: Send disabled by config\n")
				} else {
					for _, message := range messages {
						if transceiver, err := w.GetTransceiver(); err == nil {
							messageId, err := transceiver.SendMessage(message.From, message.Phone, message.Text)
							if err == nil {
								message.MessageId = messageId
								message.Status = "sent"
								w.db.Conn.Save(&message)
								log.Printf("Info: SendMessage OK from=%q dest=%q text=%q\n", message.From, message.Phone, message.Text)
							} else {
								message.Status = "errored"
								message.TryCount += 1
								message.LastError = err.Error()
								w.db.Conn.Save(&message)
								log.Printf("Info: SendMessage ERROR: from=%q dest=%q text=%q: %s\n",
									message.From, message.Phone, message.Text, err.Error())
								break
							}
						} else {
							log.Printf("Error: %s\n", err.Error())
						}
					}
				}
			} else {
				log.Printf("Error: %s\n", err.Error())
			}
		}
	}()
	go func() {
		time.Sleep(time.Second * 2)
		for {
			w.Flush()
			time.Sleep(SendInterval)
		}
	}()
}

func (w *SMPPWorker) Flush() {
	select {
	case w.FlushCh <- true:
	default:
	}
}
